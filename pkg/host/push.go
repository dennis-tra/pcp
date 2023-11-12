package host

import (
	"context"
	"fmt"
	stdio "io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dennis-tra/pcp/pkg/tui"
	"github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/io"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

const ProtocolPushRequest = "/pcp/push/0.0.1"

type PushState string

const (
	// sender
	PushStateIdle             PushState = "idle"
	PushStateSendingRequest   PushState = "sending_request"
	PushStateAwaitingResponse PushState = "asked_peer"

	// receiver
	PushStateOpenedStream    PushState = "opened_stream"
	PushStateSendingResponse PushState = "sending_response"

	// both
	PushStateError    PushState = "error"
	PushStateAccepted PushState = "accepted"
	PushStateRejected PushState = "rejected"
)

// PushProtocol is the protocol that's responsible for exchanging information
// about the data that's about to be transmitted and asking for confirmation
// to start the transfer
type PushProtocol struct {
	ctx    context.Context
	host   host.Host
	sender tea.Sender
	state  PushState
	peer   peer.ID
	role   discovery.Role
	list   list.Model
	err    error

	spinner    spinner.Model
	respStream network.Stream
}

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
)

type item string

func (i item) FilterValue() string { return "" }

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w stdio.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

func NewPushProtocol(ctx context.Context, host host.Host, sender tea.Sender, role discovery.Role) *PushProtocol {
	return &PushProtocol{
		ctx:     ctx,
		host:    host,
		sender:  sender,
		state:   PushStateIdle,
		role:    role,
		peer:    "",
		list:    list.New([]list.Item{item("Yes"), item("No")}, itemDelegate{}, 20, 14),
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
}

type pushOnSendRequest struct {
	stream network.Stream
}

func (p *PushProtocol) RegisterPushRequestHandler(pid peer.ID) {
	log.WithField("peer", pid.String()).Debugln("Registering push request handler")
	p.peer = pid
	p.host.SetStreamHandler(ProtocolPushRequest, func(s network.Stream) {
		p.sender.Send(pushOnSendRequest{stream: s})
	})
}

func (p *PushProtocol) UnregisterPushRequestHandler() {
	log.Debugln("Unregistering push request handler")
	p.host.RemoveStreamHandler(ProtocolPushRequest)
}

func (p *PushProtocol) Init() tea.Cmd {
	return p.spinner.Tick
}

func (p *PushProtocol) Update(msg tea.Msg) (*PushProtocol, tea.Cmd) {
	log.WithFields(logrus.Fields{
		"comp": "push",
		"type": fmt.Sprintf("%T", msg),
	}).Tracef("handle message: %T\n", msg)

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case pushOnSendRequest:
		// Only allow authenticated peers to talk to us
		if p.peer != msg.stream.Conn().RemotePeer() {
			log.Warnln("Received push request from unauthenticated peer")
			// Tell peer to go away
			if err := msg.stream.Reset(); err != nil {
				log.WithError(err).Warnln("failed resetting push stream")
			}
		} else {
			p, cmd = p.handlePushRequest(msg.stream)
			cmds = append(cmds, cmd)
		}
	case pushMsg[*p2p.PushResponse]:
		if err := msg.stream.Close(); err != nil {
			log.WithError(err).Warnln("error closing push stream")
		}

		if msg.payload.Accept {
			p.state = PushStateAccepted
		} else {
			p.state = PushStateRejected
		}

	case pushMsg[*p2p.PushRequest]:
		switch p.role {
		case discovery.RoleReceiver:
			p.state = PushStateAwaitingResponse
			p.respStream = msg.stream
		case discovery.RoleSender:
			p, cmd = p.awaitResponse(msg.stream)
			cmds = append(cmds, cmd)
		}
	case pushMsg[error]:
		p.state = PushStateError
		p.err = msg.payload
		cmds = append(cmds, Shutdown)
	case tea.KeyMsg:
		if p.role == discovery.RoleReceiver && p.state == PushStateAwaitingResponse {
			switch keypress := msg.String(); keypress {
			case "enter":
				switch p.list.SelectedItem().(item) {
				case "Yes":
					cmd = p.sendPushResponse(p.respStream, true)
					cmds = append(cmds, cmd)
				case "No":
					cmd = p.sendPushResponse(p.respStream, false)
					cmds = append(cmds, cmd)
				}
				return p, tea.Quit
			}
		}
	}

	p.list, cmd = p.list.Update(msg)
	cmds = append(cmds, cmd)

	p.spinner, cmd = p.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return p, tea.Batch(cmds...)
}

func (p *PushProtocol) View() string {
	out := ""

	switch p.role {
	case discovery.RoleSender:
		switch p.state {
		case PushStateIdle:
		case PushStateSendingRequest:
			out += p.spinner.View() + "sending confirmation request..."
		case PushStateAwaitingResponse:
			out += p.spinner.View() + "asking peer for confirmation..."
		case PushStateAccepted:
			out += "asking peer for confirmation... " + tui.Green.Render("Accepted!")
		case PushStateRejected:
			out += "asking peer for confirmation... " + tui.Red.Render("Rejected!")
		case PushStateError:
			out += "an error occurred while asking for confirmation: " + p.err.Error()
		}
	case discovery.RoleReceiver:
		switch p.state {
		case PushStateIdle:
		case PushStateOpenedStream:
			out += p.spinner.View() + "reading confirmation request..."
		case PushStateAwaitingResponse:
			out += "\n" + p.list.View()
		case PushStateSendingResponse:
			out += p.spinner.View() + "sending response..."
		}
	}

	return out
}

func basePushErrMsg(stream network.Stream, pid peer.ID) func(err error) tea.Msg {
	return func(err error) tea.Msg {
		return pushMsg[error]{stream: stream, pid: pid, payload: err}
	}
}

type pushMsg[T any] struct {
	pid     peer.ID
	payload T
	stream  network.Stream
}

func (p *PushProtocol) SendPushRequest(peerID peer.ID, filename string, size int64, isDir bool) tea.Cmd {
	p.state = PushStateSendingRequest
	return func() tea.Msg {
		s, err := p.host.NewStream(p.ctx, peerID, ProtocolPushRequest)
		if err != nil {
			return pushMsg[error]{pid: peerID, payload: err}
		}

		defer s.Close()
		defer io.ResetOnShutdown(p.ctx, s)()

		pushErrMsg := basePushErrMsg(s, s.Conn().RemotePeer())

		log.Debugln("Sending push request", filename, size)

		req := p2p.NewPushRequest(filename, size, isDir)
		data, err := proto.Marshal(req)
		if err != nil {
			return pushErrMsg(err)
		}

		if _, err := io.WriteBytes(s, data); err != nil {
			return pushErrMsg(err)
		}

		return pushMsg[*p2p.PushRequest]{
			pid:     s.Conn().RemotePeer(),
			payload: req,
			stream:  s,
		}
	}
}

func (p *PushProtocol) awaitResponse(s network.Stream) (*PushProtocol, tea.Cmd) {
	p.state = PushStateAwaitingResponse
	return p, func() tea.Msg {
		pushErrMsg := basePushErrMsg(s, s.Conn().RemotePeer())
		data, err := io.ReadBytes(s)
		if err != nil {
			return pushErrMsg(err)
		}

		resp := &p2p.PushResponse{}
		if err := proto.Unmarshal(data, resp); err != nil {
			return pushErrMsg(err)
		}

		return pushMsg[*p2p.PushResponse]{
			pid:     s.Conn().RemotePeer(),
			payload: resp,
			stream:  s,
		}
	}
}

func (p *PushProtocol) handlePushRequest(s network.Stream) (*PushProtocol, tea.Cmd) {
	p.state = PushStateOpenedStream
	return p, func() tea.Msg {
		defer s.Close()
		defer io.ResetOnShutdown(p.ctx, s)()

		pushErrMsg := basePushErrMsg(s, s.Conn().RemotePeer())

		data, err := io.ReadBytes(s)
		if err != nil {
			return pushErrMsg(err)
		}

		req := &p2p.PushRequest{}
		if err := proto.Unmarshal(data, req); err != nil {
			return pushErrMsg(err)
		}

		log.Debugln("Received push request", req.Name, req.Size)
		return pushMsg[*p2p.PushRequest]{
			pid:     s.Conn().RemotePeer(),
			payload: req,
			stream:  s,
		}
	}
}

func (p *PushProtocol) sendPushResponse(s network.Stream, accept bool) tea.Cmd {
	return func() tea.Msg {
		defer s.Close()

		pushErrMsg := basePushErrMsg(s, s.Conn().RemotePeer())

		resp := p2p.NewPushResponse(accept)
		data, err := proto.Marshal(resp)
		if err != nil {
			return pushErrMsg(err)
		}

		if _, err := io.WriteBytes(s, data); err != nil {
			return pushErrMsg(err)
		}

		if err := io.WaitForEOF(p.ctx, s); err != nil {
			return pushErrMsg(err)
		}

		return nil
	}
}
