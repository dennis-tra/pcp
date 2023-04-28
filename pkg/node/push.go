package node

import (
	"context"
	"sync"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/dennis-tra/pcp/internal/log"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// pattern: /protocol-name/request-or-response-message/version
const ProtocolPushRequest = "/pcp/push/0.0.1"

// PushProtocol is the protocol that's responsible for exchanging information
// about the data that's about to be transmitted and asking for confirmation
// to start the transfer
type PushProtocol struct {
	node   *Node
	lk     sync.RWMutex
	prh    PushRequestHandler
	locker sync.Map
}

type PushRequestHandler interface {
	HandlePushRequest(*p2p.PushRequest) (bool, error)
}

func NewPushProtocol(node *Node) PushProtocol {
	return PushProtocol{
		node:   node,
		lk:     sync.RWMutex{},
		locker: sync.Map{},
	}
}

func (p *PushProtocol) RegisterPushRequestHandler(prh PushRequestHandler) {
	log.Debugln("Registering push request handler")
	p.lk.Lock()
	defer p.lk.Unlock()

	p.prh = prh
	p.node.SetStreamHandler(ProtocolPushRequest, p.onPushRequest)
}

func (p *PushProtocol) UnregisterPushRequestHandler() {
	log.Debugln("Unregistering push request handler")
	p.lk.Lock()
	defer p.lk.Unlock()

	p.node.RemoveStreamHandler(ProtocolPushRequest)
	p.prh = nil
}

func (p *PushProtocol) LockHandler(peerID peer.ID) {
	log.Debugln("Lock push request handler")
	p.locker.LoadOrStore(peerID, make(chan struct{}))
}

func (p *PushProtocol) UnlockHandler(peerID peer.ID) {
	log.Debugln("Unlock push request handler")
	lock, loaded := p.locker.LoadAndDelete(peerID)
	if !loaded {
		return
	}

	close(lock.(chan struct{}))
}

func (p *PushProtocol) onPushRequest(s network.Stream) {
	defer s.Close()
	defer p.node.ResetOnShutdown(s)()

	// Only allow authenticated peers to talk to us
	if !p.node.IsAuthenticated(s.Conn().RemotePeer()) {
		log.Infoln("Received push request from unauthenticated peer")
		s.Reset() // Tell peer to go away
		return
	}

	// wait until this handler is allowed to proceed.
	lock, found := p.locker.Load(s.Conn().RemotePeer())
	if found {
		select {
		case <-p.node.ServiceContext().Done():
			return
		case <-lock.(chan struct{}):
		}
	}

	req := &p2p.PushRequest{}
	if err := p.node.Read(s, req); err != nil {
		log.Infoln(err)
		return
	}
	log.Debugln("Received push request", req.Name, req.Size)

	p.lk.RLock()
	defer p.lk.RUnlock()
	accept, err := p.prh.HandlePushRequest(req)
	if err != nil {
		log.Infoln(err)
		accept = false
		// Fall through and tell peer we won't handle the request
	}

	if err := p.node.Send(s, p2p.NewPushResponse(accept)); err != nil {
		log.Infoln(err)
		return
	}

	if err = s.CloseWrite(); err != nil {
		log.Warningln("Error closing writer side of stream:", err)
	}

	if err = p.node.WaitForEOF(s); err != nil {
		log.Infoln(err)
		return
	}
}

func (p *PushProtocol) SendPushRequest(ctx context.Context, peerID peer.ID, filename string, size int64, isDir bool) (bool, error) {
	s, err := p.node.NewStream(ctx, peerID, ProtocolPushRequest)
	if err != nil {
		return false, err
	}
	defer s.Close()

	log.Debugln("Sending push request", filename, size)
	if err = p.node.Send(s, p2p.NewPushRequest(filename, size, isDir)); err != nil {
		return false, err
	}

	resp := &p2p.PushResponse{}
	if err = p.node.Read(s, resp); err != nil {
		return false, err
	}

	return resp.Accept, nil
}
