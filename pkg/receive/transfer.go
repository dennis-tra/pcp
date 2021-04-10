package receive

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"

	progress "github.com/schollz/progressbar/v3"

	"github.com/dennis-tra/pcp/internal/log"
)

type TransferHandler struct {
	filename string
	received int64
	done     chan int64
}

func NewTransferHandler(filename string, done chan int64) (*TransferHandler, error) {
	return &TransferHandler{filename: filename, done: done}, nil
}

func (th *TransferHandler) Done() {
	th.done <- th.received
	close(th.done)
}

func (th *TransferHandler) HandleFile(hdr *tar.Header, src io.Reader) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Warningln("error determining current working directory:", err)
		cwd = "."
	}

	finfo := hdr.FileInfo()
	joined := filepath.Join(cwd, hdr.Name)
	if finfo.IsDir() {
		err := os.MkdirAll(joined, finfo.Mode())
		if err != nil {
			log.Warningln("error creating directory:", joined, err)
		}
	}
	newFile, err := os.OpenFile(joined, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, finfo.Mode().Perm())
	if err != nil {
		log.Warningln("error creating file:", joined, err)
		return
	}

	bar := progress.DefaultBytes(hdr.Size, filepath.Base(hdr.Name))
	n, err := io.Copy(io.MultiWriter(newFile, bar), src)
	th.received += n
	if err != nil {
		log.Warningln("error writing file content:", joined, err)
		return
	}
}
