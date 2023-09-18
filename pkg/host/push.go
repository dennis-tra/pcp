package host

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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

// PushProtocol is the protocol that's responsible for exchanging information
// about the data that's about to be transmitted and asking for confirmation
// to start the transfer
type PushProtocol struct {
	ctx    context.Context
	host   host.Host
	sender tea.Sender
	peer   peer.ID
	role   discovery.Role
	list   list.Model
}

func NewPushProtocol(ctx context.Context, host host.Host, sender tea.Sender, role discovery.Role) *PushProtocol {
	return &PushProtocol{
		ctx:    ctx,
		host:   host,
		sender: sender,
		role:   role,
		peer:   "",
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

	case pushMsg[*p2p.PushRequest]:
		switch p.role {
		case discovery.RoleReceiver:
			// ask user
		case discovery.RoleSender:
			p, cmd = p.awaitResponse(msg.stream)
			cmds = append(cmds, cmd)
		}
	case pushMsg[error]:
		// error
	}

	p.list, cmd = p.list.Update(msg)
	cmds = append(cmds, cmd)

	return p, tea.Batch(cmds...)
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

// sending request
// sent request waiting
// received response

// received request
func (p *PushProtocol) SendPushRequest(peerID peer.ID, filename string, size int64, isDir bool) tea.Cmd {
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
		//// Asking user if they want to accept
		//accept, err := p.prh.HandlePushRequest(req)
		//if err != nil {
		//	log.Warningln(err)
		//	accept = false
		//	// Fall through and tell peer we won't handle the request
		//}
		//
		//if err = p.host.Send(s, p2p.NewPushResponse(accept)); err != nil {
		//	log.Warningln(fmt.Errorf("send push response: %w", err))
		//	p.host.Shutdown()
		//	return
		//}
		//
		//if err = p.host.WaitForEOF(s); err != nil {
		//	log.Warningln(fmt.Errorf("wait EOF: %w", err))
		//	p.host.Shutdown()
		//	return
		//}
		//
		//if !accept {
		//	p.host.Shutdown()
		//}
		return nil
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
