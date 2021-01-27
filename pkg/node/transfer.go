package node

import (
	"context"
	"fmt"
	"io"

	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

// pattern: /protocol-name/request-or-response-message/version
const transfer = "/pcp/transfer/0.0.1"
const transferAck = "/pcp/transferAck/0.0.1"

// TransferProtocol type
type TransferProtocol struct {
	node   *Node
	size   int64
	writer io.Writer
	peerId peer.ID
	//contentId   *cid.Cid
	ackChan     chan int64
	receiveChan chan int64
}

func NewTransferProtocol(node *Node) *TransferProtocol {
	p := &TransferProtocol{
		node: node,
	}
	node.SetStreamHandler(transfer, p.onTransfer)
	node.SetStreamHandler(transferAck, p.onTransferAck)
	return p
}

func (t *TransferProtocol) ExpectsData() bool {
	return t.size != 0 && t.writer != nil && t.peerId != "" && t.receiveChan != nil // && t.contentId != nil
}

func (t *TransferProtocol) SetExpectedData(peerId peer.ID, reqData *p2p.PushRequest, writer io.Writer) chan int64 {
	t.size = reqData.FileSize
	t.writer = writer
	t.peerId = peerId
	//t.contentId = reqData.Cid
	t.receiveChan = make(chan int64)
	return t.receiveChan
}

func (t *TransferProtocol) ResetExpectedData() {
	t.size = 0
	t.writer = nil
	t.peerId = ""
	//t.contentId = nil
	t.receiveChan = nil
}

func (t *TransferProtocol) onTransfer(s network.Stream) {
	if !t.ExpectsData() {
		fmt.Println("Received data transfer without expecting data")
		return
	}

	if t.peerId != s.Conn().RemotePeer() {
		fmt.Println("Received data transfer attempt from unexpected peer")
		return
	}

	// TODO: Limit by file size
	// TODO: Progress bar
	fmt.Print("Copying data...")
	r := io.LimitReader(s, t.size)
	received, err := io.Copy(t.writer, r)
	defer func() {
		t.receiveChan <- received
	}()

	if err != nil {
		fmt.Println(err)
		// TODO: Send ack with received bytes to potentially resume transfer later.
		return
	}

	fmt.Println("Done!")
	fmt.Print("Sending acknowledge to peer...")
	ack := &p2p.TransferAcknowledge{ReceivedBytes: received}
	_, err = t.node.SendProto(context.Background(), s.Conn().RemotePeer(), ack)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = s.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Done!")
}

func (t *TransferProtocol) onTransferAck(s network.Stream) {
	if t.ackChan == nil {
		fmt.Println("Received ack without waiting for an ack.")
	}

	data := &p2p.TransferAcknowledge{}
	err := t.node.readMessage(s, data)
	if err != nil {
		fmt.Println(err)
		return
	}

	t.ackChan <- data.ReceivedBytes
	close(t.ackChan)
	t.ackChan = nil

	err = s.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
}

func (t *TransferProtocol) Transfer(ctx context.Context, peerId peer.ID, payload io.Reader) (chan int64, error) {

	s, err := t.node.NewStream(ctx, peerId, transfer)
	if err != nil {
		return nil, err
	}

	t.ackChan = make(chan int64)

	_, err = io.Copy(s, payload)
	if err != nil {
		return nil, err
	}

	return t.ackChan, nil
}
