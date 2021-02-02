package proto

import (
	"github.com/golang/protobuf/proto"
	"github.com/ipfs/go-cid"
)

type HeaderMessage interface {
	GetHeader() *Header
	SetHeader(*Header)
	proto.Message
}

func (x *PushRequest) SetHeader(hdr *Header) {
	x.Header = hdr
}

func (x *PushResponse) SetHeader(hdr *Header) {
	x.Header = hdr
}

func (x *TransferAcknowledge) SetHeader(hdr *Header) {
	x.Header = hdr
}

func NewPushResponse(accept bool) *PushResponse {
	return &PushResponse{Accept: accept}
}

func NewPushRequest(fileName string, fileSize int64, c cid.Cid) *PushRequest {
	return &PushRequest{
		FileName: fileName,
		FileSize: fileSize,
		Cid:      c.Bytes(),
	}
}

func NewTransferAcknowledge(receivedBytes int64) *TransferAcknowledge {
	return &TransferAcknowledge{
		ReceivedBytes: receivedBytes,
	}
}
