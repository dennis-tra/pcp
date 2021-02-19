package receive

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/progress"
)

type TransferHandler struct {
	filename string
	size     int64
	done     chan int64
}

func NewTransferHandler(filename string, size int64, done chan int64) (*TransferHandler, error) {
	th := &TransferHandler{
		filename: filename,
		size:     size,
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

	bar := progress.DefaultBytes(th.size, th.filename)
	// Receive and persist the actual data.
	received, err = io.Copy(io.MultiWriter(f, bar), src)
	if err != nil {
		log.Infoln(errors.Wrap(err, "error receiving or writing bytes"))
	}
}

func (th *TransferHandler) GetLimit() int64 {
	return th.size
}
