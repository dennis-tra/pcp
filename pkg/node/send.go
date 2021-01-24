package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/ipfs/go-cid"

	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

// pattern: /protocol-name/request-or-response-message/version
const sendRequest = "/pcp/sendRequest/0.0.1"
const sendResponse = "/pcp/sendResponse/0.0.1"

// SendProtocol type
type SendProtocol struct {
	node      *Node
	respChans map[string]chan *SendResponseData
	ReqChan   chan *SendRequestData
}

type SendRequestData struct {
	Header    *p2p.Header
	Request   *p2p.SendRequest
	PeerId    peer.ID
	ContentId *cid.Cid
}

type SendResponseData struct {
	Header   *p2p.Header
	Response *p2p.SendResponse
}

func NewSendProtocol(node *Node) *SendProtocol {
	p := &SendProtocol{
		node:      node,
		respChans: make(map[string]chan *SendResponseData),
		ReqChan:   make(chan *SendRequestData),
	}
	node.SetStreamHandler(sendRequest, p.onSendRequest)
	node.SetStreamHandler(sendResponse, p.onSendResponse)
	return p
}

func (p *SendProtocol) onSendRequest(s network.Stream) {
	msg, err := p.node.readMessage(s)
	if err != nil {
		fmt.Println(err)
		return
	}

	req := msg.GetSendRequest()
	if req == nil {
		fmt.Println("unexpected message")
		return
	}

	contentId, err := cid.Cast(req.Cid)
	if err != nil {
		fmt.Println("could not cast cid:", hex.EncodeToString(req.Cid))
		return
	}

	chanData := &SendRequestData{
		Header:    msg,
		Request:   req,
		PeerId:    s.Conn().RemotePeer(),
		ContentId: &contentId,
	}

	p.ReqChan <- chanData
}

// remote send response handler
func (p *SendProtocol) onSendResponse(s network.Stream) {
	hdr, err := p.node.readMessage(s)
	if err != nil {
		fmt.Println(err)
		return
	}

	resp := hdr.GetSendResponse()
	if resp == nil {
		fmt.Println("unexpected message")
		return
	}

	respChan, found := p.respChans[hdr.OriginId]
	if !found {
		fmt.Println("couldn't find respChans channel for origin id", hdr.OriginId)
		return
	}

	chanData := &SendResponseData{
		Header:   hdr,
		Response: resp,
	}


	respChan <- chanData

	close(respChan)
	delete(p.respChans, hdr.OriginId) // TODO: Is this thread safe?
}

func (p *SendProtocol) SendRequest(ctx context.Context, peerId peer.ID, req *p2p.SendRequest) (chan *SendResponseData, error) {
	hdr, err := p.node.NewHeader()
	if err != nil {
		return nil, err
	}
	hdr.Payload = &p2p.Header_SendRequest{SendRequest: req}

	err = p.node.SendProto(ctx, peerId, sendRequest, hdr)
	if err != nil {
		return nil, err
	}

	respChan := make(chan *SendResponseData)
	p.respChans[hdr.RequestId] = respChan // TODO: Is this thread safe?

	return respChan, nil
}

func (p *SendProtocol) SendResponse(ctx context.Context, reqCtx *SendRequestData, resp *p2p.SendResponse) error {
	hdr, err := p.node.NewHeader()
	if err != nil {
		return err
	}

	hdr.OriginId = reqCtx.Header.RequestId
	hdr.Payload = &p2p.Header_SendResponse{SendResponse: resp}

	return p.node.SendProto(ctx, reqCtx.PeerId, sendResponse, hdr)
}
