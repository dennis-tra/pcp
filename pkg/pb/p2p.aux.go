package proto

import (
	"github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/peer"
)

type HeaderMessage interface {
	GetHeader() *Header
	SetHeader(*Header)
	PeerID() (peer.ID, error)
	proto.Message
}

func (x *PushRequest) SetHeader(hdr *Header) {
	x.Header = hdr
}

func (x *PushResponse) SetHeader(hdr *Header) {
	x.Header = hdr
}

func (x *PushRequest) PeerID() (peer.ID, error) {
	return peer.Decode(x.GetHeader().NodeId)
}

func (x *PushResponse) PeerID() (peer.ID, error) {
	return peer.Decode(x.GetHeader().NodeId)
}

func NewPushResponse(accept bool) *PushResponse {
	return &PushResponse{Accept: accept}
}

func NewPushRequest(name string, size int64, isDir bool) *PushRequest {
	return &PushRequest{
		Name:  name,
		Size:  size,
		IsDir: isDir,
	}
}
