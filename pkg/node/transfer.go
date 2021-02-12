package node

import (
	"context"
	"golang.org/x/term"
	"io"
	"os"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/dennis-tra/pcp/internal/format"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/progress"
)

// pattern: /protocol-name/request-or-response-message/version
const (
	ProtocolTransfer = "/pcp/transfer/0.1.0"
)

// TransferProtocol encapsulates data necessary to fulfill its protocol.
type TransferProtocol struct {
	node *Node
	lk   sync.RWMutex
	th   TransferHandler
}

type TransferHandler interface {
	HandleTransfer(r io.Reader)
	GetLimit() int64
}

func (t *TransferProtocol) RegisterTransferHandler(th TransferHandler) {
	t.lk.Lock()
	defer t.lk.Unlock()
	t.th = th
	t.node.SetStreamHandler(ProtocolTransfer, t.onTransfer)
}

func (t *TransferProtocol) UnregisterTransferHandler() {
	t.lk.Lock()
	defer t.lk.Unlock()
	t.node.RemoveStreamHandler(ProtocolTransfer)
	t.th = nil
}

// New TransferProtocol initializes a new TransferProtocol object with all
// fields set to their default values.
func NewTransferProtocol(node *Node) *TransferProtocol {
	return &TransferProtocol{node: node, lk: sync.RWMutex{}}
}

// onTransfer is called when the peer initiates a file transfer.
func (t *TransferProtocol) onTransfer(s network.Stream) {

	if !t.node.IsAuthenticated(s.Conn().RemotePeer()) {
		log.Infoln("Received push request from unauthenticated peer")
		return
	}

	t.lk.RLock()
	defer func() {
		if err := s.Close(); err != nil {
			log.Infoln(err)
		}
		t.lk.RUnlock()
	}()

	// Only read as much as we expect to avoid stuffing.
	lr := io.LimitReader(s, t.th.GetLimit())

	t.th.HandleTransfer(lr)
}

// Transfer can be called to transfer the given payload to the given peer. The PushRequest is used for displaying
// the progress to the user. This function returns when the bytes where transmitted and we have received an
// acknowledgment.
func (t *TransferProtocol) Transfer(ctx context.Context, peerID peer.ID, payload io.Reader) (int64, error) {

	// Open a new stream to our peer.
	s, err := t.node.NewStream(ctx, peerID, ProtocolTransfer)
	if err != nil {
		return 0, err
	}
	defer s.Close()

	// The actual file transfer.
	written, err := io.Copy(s, payload)
	if err != nil {
		return 0, err
	}

	return written, t.node.WaitForEOF(s)
}

func IndicateProgress(ctx context.Context, bCounter progress.Counter, filename string, size int64, wg *sync.WaitGroup) {

	ticker := progress.NewTicker(ctx, bCounter, size, 500*time.Millisecond)
	tWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		tWidth = 80
	}

	iCounter := 0 // iteration counter
	start := time.Now()

	for t := range ticker {
		bps := int64(float64(t.N()) / time.Now().Sub(start).Seconds()) // bytes per second
		msg := format.TransferStatus(filename, iCounter, tWidth, t.Percent()/100, t.Remaining(), bps)
		log.Infof("\r%s", msg)
		iCounter++
	}

	wg.Done()
}
