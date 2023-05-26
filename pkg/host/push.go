package host

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/io"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// pattern: /protocol-name/request-or-response-message/version
const ProtocolPushRequest = "/pcp/push/0.0.1"

// PushProtocol is the protocol that's responsible for exchanging information
// about the data that's about to be transmitted and asking for confirmation
// to start the transfer
type PushProtocol struct {
	ctx     context.Context
	host    *Host
	program *tea.Program
}

func NewPushProtocol(ctx context.Context, host *Host, program *tea.Program) PushProtocol {
	return PushProtocol{
		ctx:     ctx,
		host:    host,
		program: program,
	}
}

func (p *PushProtocol) RegisterPushRequestHandler() {
	log.Debugln("Registering push request handler")
	p.host.SetStreamHandler(ProtocolPushRequest, func(s network.Stream) {
		p.program.Send(pakeOnKeyExchange(s))
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

	switch msg := msg.(type) {
	case pushRequest:
		stream := msg

		// Only allow authenticated peers to talk to us
		if !p.host.IsAuthenticated(stream.Conn().RemotePeer()) {
			log.Infoln("Received push request from unauthenticated peer")
			stream.Reset() // Tell peer to go away
			return p, nil
		}
	}
	return p, nil
}

type pushRequest network.Stream

func (p *PushProtocol) handlePushRequest(s network.Stream) tea.Cmd {
	return func() tea.Msg {
		//defer io.ResetOnShutdown(p.ctx, s)()
		//
		//req := &p2p.PushRequest{}
		//if err := p.host.Read(s, req); err != nil {
		//	log.Warningln(err)
		//	return nil
		//}
		//return req
		//
		//log.Debugln("Received push request", req.Name, req.Size)
		//
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

func (p *PushProtocol) SendPushResponse(s network.Stream, accept bool) tea.Cmd {
	return func() tea.Msg {
		defer s.Close()

		if err := p.host.Send(s, p2p.NewPushResponse(accept)); err != nil {
			return nil
		}

		if err := io.WaitForEOF(p.ctx, s); err != nil {
			log.Warningln(fmt.Errorf("wait EOF: %w", err))
			return nil
		}

		return nil
	}
}

func (p *PushProtocol) SendPushRequest(ctx context.Context, peerID peer.ID, filename string, size int64, isDir bool) (bool, error) {
	s, err := p.host.NewStream(ctx, peerID, ProtocolPushRequest)
	if err != nil {
		return false, fmt.Errorf("new stream: %w", err)
	}
	defer s.Close()
	defer io.ResetOnShutdown(p.ctx, s)()

	log.Debugln("Sending push request", filename, size)
	if err = p.host.Send(s, p2p.NewPushRequest(filename, size, isDir)); err != nil {
		return false, fmt.Errorf("send: %w", err)
	}

	resp := &p2p.PushResponse{}
	if err = p.host.Read(s, resp); err != nil {
		return false, fmt.Errorf("read: %w", err)
	}

	return resp.Accept, nil
}
