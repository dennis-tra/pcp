package receive

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/node"
	"github.com/dennis-tra/pcp/pkg/progress"
)

type TransferHandler struct {
	peerID   peer.ID
	filename string
	size     int64
	cid      []byte
	done     chan int64
}

func NewTransferHandler(peerID peer.ID, filename string, size int64, cid []byte, done chan int64) (*TransferHandler, error) {

	th := &TransferHandler{
		peerID:   peerID,
		filename: filename,
		size:     size,
		cid:      cid,
		done:     done,
	}

	return th, nil
}

func (th *TransferHandler) HandleTransfer(src io.Reader) {
	var received int64
	defer func() {
		th.done <- received
		close(th.done)
	}()

	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	filename := filepath.Base(th.filename)

	log.Infoln("Saving file to: ", filepath.Join(cwd, filename))
	f, err := os.Create(filepath.Join(cwd, filename))
	if err != nil {
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Infoln(err)
		}
	}()

	pw := progress.NewWriter(f)

	var wg sync.WaitGroup
	wg.Add(1)

	ctx, cancel := context.WithCancel(context.Background())
	go node.IndicateProgress(ctx, pw, th.filename, th.size, &wg)

	// Receive and persist the actual data.
	received, err = io.Copy(pw, src)
	cancel()
	wg.Wait()

	if err != nil {
		log.Infoln(errors.Wrap(err, "error receiving or writing bytes"))
	}
}

func (th *TransferHandler) GetLimit() int64 {
	return th.size
}

func (th *TransferHandler) GetPeerID() peer.ID {
	return th.peerID
}
