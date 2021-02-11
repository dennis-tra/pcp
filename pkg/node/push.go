package node

import (
	"context"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/dennis-tra/pcp/internal/log"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// pattern: /protocol-name/request-or-response-message/version
const ProtocolPushRequest = "/pcp/push/0.0.1"

// PushProtocol .
type PushProtocol struct {
	node *Node
	lk   sync.RWMutex
	prh  PushRequestHandler
}

type PushRequestHandler interface {
	HandlePushRequest(*p2p.PushRequest) (bool, error)
}

func NewPushProtocol(node *Node) *PushProtocol {
	return &PushProtocol{node: node, lk: sync.RWMutex{}}
}

func (p *PushProtocol) RegisterPushRequestHandler(prh PushRequestHandler) {
	p.lk.Lock()
	defer p.lk.Unlock()
	p.prh = prh
	p.node.SetStreamHandler(ProtocolPushRequest, p.onPushRequest)
}

func (p *PushProtocol) UnregisterPushRequestHandler() {
	p.lk.Lock()
	defer p.lk.Unlock()
	p.node.RemoveStreamHandler(ProtocolPushRequest)
	p.prh = nil
}

func (p *PushProtocol) onPushRequest(s network.Stream) {
	defer s.Close()

	if !p.node.IsAuthenticated(s.Conn().RemotePeer()) {
		log.Infoln("Received push request from unauthenticated peer")
		return
	}

	req := &p2p.PushRequest{}
	if err := p.node.Read(s, req); err != nil {
		log.Infoln(err)
		return
	}

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

	if err = p.node.WaitForEOF(s); err != nil {
		log.Infoln(err)
		return
	}
}

func (p *PushProtocol) SendPushRequest(ctx context.Context, peerID peer.ID, filename string, size int64, c cid.Cid) (bool, error) {

	s, err := p.node.NewStream(ctx, peerID, ProtocolPushRequest)
	if err != nil {
		return false, err
	}
	defer s.Close()

	if err = p.node.Send(s, p2p.NewPushRequest(filename, size, c)); err != nil {
		return false, err
	}

	resp := &p2p.PushResponse{}
	if err = p.node.Read(s, resp); err != nil {
		return false, err
	}

	return resp.Accept, nil
}
