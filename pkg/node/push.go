package node

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// pattern: /protocol-name/request-or-response-message/version
const ProtocolPushRequest = "/pcp/pushRequest/0.0.1"
const ProtocolPushResponse = "/pcp/pushResponse/0.0.1"

// PushProtocol type
type PushProtocol struct {
	node      *Node
	respChans sync.Map
	reqChans  []chan PushRequest
}

type PushRequest struct {
	Message *p2p.PushRequest
	PeerId  peer.ID
}

func NewPushProtocol(node *Node) *PushProtocol {
	p := &PushProtocol{
		node:      node,
		respChans: sync.Map{},
		reqChans:  []chan PushRequest{},
	}
	node.SetStreamHandler(ProtocolPushRequest, p.onPushRequest)
	node.SetStreamHandler(ProtocolPushResponse, p.onPushResponse)
	return p
}

func (p *PushProtocol) WaitForPushRequest() (peer.ID, *p2p.PushRequest) {
	c := make(chan PushRequest, 1)
	p.reqChans = append(p.reqChans, c)

	chanMsg := <-c

	return chanMsg.PeerId, chanMsg.Message
}

func (p *PushProtocol) onPushRequest(s network.Stream) {

	data := &p2p.PushRequest{}
	err := p.node.readMessage(s, data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	for _, c := range p.reqChans {
		c <- PushRequest{
			Message: data,
			PeerId:  s.Conn().RemotePeer(),
		}
		close(c)
	}
	p.node.reqChans = []chan PushRequest{}
}

// remote push response handler
func (p *PushProtocol) onPushResponse(s network.Stream) {

	data := &p2p.PushResponse{}
	err := p.node.readMessage(s, data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	respChanObj, found := p.respChans.LoadAndDelete(data.Header.RequestParentId)
	if !found {
		fmt.Fprintln(os.Stderr, "couldn't find respChans channel for origin id", data.Header.RequestParentId)
		return
	}
	respChan := respChanObj.(chan *p2p.PushResponse)

	respChan <- data
	close(respChan)
}

func (p *PushProtocol) SendPushRequest(ctx context.Context, peerId peer.ID, req *p2p.PushRequest) (*p2p.PushResponse, error) {
	requestId, err := p.node.SendProto(ctx, peerId, req)
	if err != nil {
		return nil, err
	}

	respChan := make(chan *p2p.PushResponse)

	p.respChans.Store(requestId, respChan)
	select {
	case <-ctx.Done():
		p.respChans.Delete(requestId)
		return nil, fmt.Errorf("context cancelled")
	case resp := <-respChan:
		return resp, nil
	}
}
