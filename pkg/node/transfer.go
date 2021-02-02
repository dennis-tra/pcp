package node

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	"github.com/dennis-tra/pcp/internal/format"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/commons"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/dennis-tra/pcp/pkg/progress"
)

// pattern: /protocol-name/request-or-response-message/version
const (
	ProtocolTransfer    = "/pcp/transfer/0.1.0"
	ProtocolTransferAck = "/pcp/transferAck/0.1.0"
)

// TransferProtocol encapsulates data necessary to fulfill its protocol.
type TransferProtocol struct {
	node *Node

	// receiveExps is a map from peerId -> expectation. This map is filled
	// when we're expecting data from a peer.
	receiveExps sync.Map

	// transferAcksChans is a map from peerId -> chan receivedBytes. This map
	// is filled with a channel when a routine thinks it has transmitted
	// all the data. The channel will be sent a message to when the receiving
	// peer has received all the data and sent an acknowledgement back.
	transferAcksChans sync.Map
}

// New TransferProtocol initializes a new TransferProtocol object with all
// fields set to their default values.
func NewTransferProtocol(node *Node) *TransferProtocol {
	p := &TransferProtocol{
		node:              node,
		receiveExps:       sync.Map{},
		transferAcksChans: sync.Map{},
	}
	node.SetStreamHandler(ProtocolTransfer, p.onTransfer)
	node.SetStreamHandler(ProtocolTransferAck, p.onTransferAck)
	return p
}

// expectation represents that a routine expects data to be received from the
// given peerId and other information. A message with the received bytes is
// sent to the received channel.
type expectation struct {
	peerId   peer.ID
	dest     io.Writer
	name     string
	size     int64
	cid      []byte
	received chan int64
}

// Receive can be called to indicate that we're awaiting a file transfer. We say from
// whom we expect the transfer (peerId), what and how much data we expect (reqData)
// and where we're gonna save the data (dest).
func (t *TransferProtocol) Receive(ctx context.Context, peerId peer.ID, reqData *p2p.PushRequest, dest io.Writer) (int64, error) {

	exp := &expectation{
		peerId:   peerId,
		dest:     dest,
		name:     reqData.FileName,
		size:     reqData.FileSize,
		cid:      reqData.Cid,
		received: make(chan int64),
	}

	// Check if there is already some other routine waiting for a file transfer
	_, loaded := t.receiveExps.LoadOrStore(peerId, exp)
	if loaded {
		return 0, fmt.Errorf("already waiting for data from peer %s", peerId)
	}
	defer t.receiveExps.Delete(peerId)

	select {
	case <-ctx.Done():
		return 0, fmt.Errorf("context cancelled")
	case received := <-exp.received:
		return received, nil
	}
}

// onTransfer is called when the peer initiates a file transfer.
func (t *TransferProtocol) onTransfer(s network.Stream) {

	// Check if we're actually waiting for data.
	obj, found := t.receiveExps.LoadAndDelete(s.Conn().RemotePeer())
	if !found {
		log.Infoln("Received data transfer attempt from unexpected peer")
		if err := s.Reset(); err != nil {
			log.Infoln("Couldn't reset stream:", err)
		}
		return
	}
	exp := obj.(*expectation)

	// Since we're wrapping only one part of the reader/writer pair it could happen
	// that in the other part an error occurs. If that's the case the IndicateProgress
	// call would not terminate. So after the copy has finished we cancel the associated
	// context and wait until the last information was written.
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)

	// Only read as much as we expect to avoid stuffing.
	lr := io.LimitReader(s, exp.size)
	pr := progress.NewReader(lr)
	go IndicateProgress(ctx, pr, exp.name, exp.size, &wg)

	// Receive and persist the actual data.
	received, err := io.Copy(exp.dest, pr)
	cancel()
	wg.Wait()

	//TODO: Verify expected content ID from peer.

	// Log any error when receiving or persisting the data.
	if err != nil {
		log.Infoln(errors.Wrap(err, "error receiving or writing bytes"))
	}

	// Acknowledge the data that we have received - even if partial. Though if
	// we only received data partially the stream is probably broken and the
	// acknowledgement won't reach its destination.
	_, err = t.node.SendProto(context.Background(), s.Conn().RemotePeer(), p2p.NewTransferAcknowledge(received))
	if err != nil {
		log.Infoln(errors.Wrap(err, "failed sending acknowledge"))
	}

	if err = s.Close(); err != nil {
		log.Infoln(errors.Wrap(err, "couldn't close stream"))
	}

	// Notify the waiting routine about the amount of received bytes.
	exp.received <- received
}

// Transfer can be called to transfer the given payload to the given peer. The PushRequest is used for displaying
// the progress to the user. This function returns when the bytes where transmitted and we have received an
// acknowledgment.
func (t *TransferProtocol) Transfer(ctx context.Context, peerId peer.ID, payload io.Reader, filename string, filesize int64) (int64, error) {

	// Check if we're already awaiting an acknowledgment from a peer.
	_, found := t.transferAcksChans.Load(peerId.String())
	if found {
		return 0, errors.New("Couldn't initiate transfer as there is already one going on")
	}

	// Create a channel on which we'll receive the acknowledgment of the transfer.
	ackChan := make(chan int64, 1)
	t.transferAcksChans.Store(peerId.String(), ackChan)
	defer t.transferAcksChans.Delete(peerId.String())

	// Open a new stream to our peer.
	s, err := t.node.NewStream(ctx, peerId, ProtocolTransfer)
	if err != nil {
		return 0, err
	}

	// Since we're wrapping only one part of the reader/writer pair it could happen
	// that in the other part an error occurs. If that's the case the IndicateProgress
	// call would not terminate. So after the copy has finished we cancel the associated
	// context and wait until the last information was written.
	cancelCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)

	// Wrapped writer to print the progress to the screen.
	pw := progress.NewWriter(s)
	go IndicateProgress(cancelCtx, pw, filename, filesize, &wg)

	// The actual file transfer.
	_, err = io.Copy(pw, payload)
	cancel()
	wg.Wait()
	if err != nil {
		log.Info(errors.Wrap(err, "Could not transfer file"))
		return 0, err
	}

	// Respond to a cancelled context as well.
	select {
	case <-ctx.Done():
		return 0, fmt.Errorf("context cancelled")
	case received := <-ackChan:
		return received, nil
	}
}

// onTransferAck is called when the peer is acknowledging the transfer of a certain amount of bytes.
func (t *TransferProtocol) onTransferAck(s network.Stream) {

	// Check whether we're waiting for an acknowledgement.
	obj, found := t.transferAcksChans.LoadAndDelete(s.Conn().RemotePeer().String())
	if !found {
		log.Infoln("Received ack without waiting for an ack.")
		err2 := s.Reset()
		if err2 != nil {
			log.Infoln(errors.Wrap(err2, "Could not reset stream"))
		}
		return
	}

	receivedBytes := int64(0)
	defer func() {
		// Notify waiting
		ackChan := obj.(chan int64)
		ackChan <- receivedBytes
		close(ackChan)
	}()

	// Read complete message
	ack := &p2p.TransferAcknowledge{}
	if err := t.node.readMessage(s, ack); err != nil {
		log.Infoln(err)
		return
	}
	receivedBytes = ack.ReceivedBytes

	if err := s.Close(); err != nil {
		log.Infoln(errors.Wrap(err, "Could not close stream"))
		return
	}
}

func IndicateProgress(ctx context.Context, counter progress.Counter, filename string, size int64, wg *sync.WaitGroup) {
	defer wg.Done()
	pChan := progress.NewTicker(ctx, counter, size, 1000*time.Millisecond)
	twidth := commons.TerminalWidth()
	iteration := 0
	started := time.Now()

	for p := range pChan {
		diff := int64(float64(p.N()) / time.Now().Sub(started).Seconds())
		bytesPerS := int64(0)
		if diff > 0 {
			bytesPerS = diff
		}

		transferStatus := format.TransferStatus(filename, iteration, twidth, p.Percent()/100, p.Remaining(), bytesPerS)
		log.Infof("\r%s", transferStatus)

		iteration++
	}
	diff := int64(float64(size) / time.Now().Sub(started).Seconds())
	bytesPerS := int64(0)
	if diff > 0 {
		bytesPerS = diff
	}
	transferStatus := format.TransferStatus(filename, 0, twidth, 1.0, 0, bytesPerS)
	log.Infof("\r%s\n", transferStatus)
}
