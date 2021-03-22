package node

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	progress "github.com/schollz/progressbar/v3"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/crypt"
)

// pattern: /protocol-name/request-or-response-message/version
const (
	ProtocolTransfer = "/pcp/transfer/0.2.0"
)

// TransferProtocol encapsulates data necessary to fulfill its protocol.
type TransferProtocol struct {
	node *Node
	lk   sync.RWMutex
	th   TransferHandler
}

type TransferHandler interface {
	HandleFile(*tar.Header, io.Reader)
	Done()
}

func (t *TransferProtocol) RegisterTransferHandler(th TransferHandler) {
	log.Debugln("Registering transfer handler")
	t.lk.Lock()
	defer t.lk.Unlock()
	t.th = th
	t.node.SetStreamHandler(ProtocolTransfer, t.onTransfer)
}

func (t *TransferProtocol) UnregisterTransferHandler() {
	log.Debugln("Unregistering transfer handler")
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
	defer t.th.Done()
	defer t.node.ResetOnShutdown(s)()

	// Get PAKE session key for stream decryption
	sKey, found := t.node.GetSessionKey(s.Conn().RemotePeer())
	if !found {
		log.Warningln("Received transfer from unauthenticated peer:", s.Conn().RemotePeer())
		s.Reset() // Tell peer to go away
		return
	}

	// Read initialization vector from stream. This is sent first from our peer.
	iv, err := t.node.ReadBytes(s)
	if err != nil {
		log.Warningln("Could not read stream initialization vector", err)
		s.Reset() // Stream is probably broken anyways
		return
	}

	t.lk.RLock()
	defer func() {
		if err = s.Close(); err != nil {
			log.Warningln(err)
		}
		t.lk.RUnlock()
	}()

	// Decrypt the stream
	sd, err := crypt.NewStreamDecrypter(sKey, iv, s)
	if err != nil {
		log.Warningln("Could not instantiate stream decrypter", err)
		return
	}

	// Drain tar archive
	tr := tar.NewReader(sd)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		} else if err != nil {
			log.Warningln("Error reading next tar element", err)
			return
		}
		t.th.HandleFile(hdr, tr)
	}

	// Read file hash from the stream and check if it matches
	hash, err := t.node.ReadBytes(s)
	if err != nil {
		log.Warningln("Could not read hash", err)
		return
	}

	// Check if hashes match
	if err = sd.Authenticate(hash); err != nil {
		log.Warningln("Could not authenticate received data", err)
		return
	}
}

// Transfer can be called to transfer the given payload to the given peer. The PushRequest is used for displaying
// the progress to the user. This function returns when the bytes where transmitted and we have received an
// acknowledgment.
func (t *TransferProtocol) Transfer(ctx context.Context, peerID peer.ID, basePath string) error {
	// Open a new stream to our peer.
	s, err := t.node.NewStream(ctx, peerID, ProtocolTransfer)
	if err != nil {
		return err
	}

	defer s.Close()
	defer t.node.ResetOnShutdown(s)()

	base, err := os.Stat(basePath)
	if err != nil {
		return err
	}

	// Get PAKE session key for stream encryption
	sKey, found := t.node.GetSessionKey(peerID)
	if !found {
		return fmt.Errorf("session key not found to encrypt data transfer")
	}

	// Initialize new stream encrypter
	se, err := crypt.NewStreamEncrypter(sKey, s)
	if err != nil {
		return err
	}

	// Send the encryption initialization vector to our peer.
	// Does not need to be encrypted, just unique.
	// https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Initialization_vector_.28IV.29
	_, err = t.node.WriteBytes(s, se.InitializationVector())
	if err != nil {
		return err
	}

	tw := tar.NewWriter(se)
	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		log.Debugln("Preparing file for transmission:", path)
		if err != nil {
			log.Debugln("Error walking file:", err)
			return err
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return errors.Wrapf(err, "error writing tar file info header %s: %s", path, err)
		}

		// To preserve directory structure in the tar ball.
		hdr.Name, err = relPath(basePath, base.IsDir(), path)
		if err != nil {
			return errors.Wrapf(err, "error building relative path: %s (%v) %s", basePath, base.IsDir(), path)
		}

		if err = tw.WriteHeader(hdr); err != nil {
			return errors.Wrap(err, "error writing tar header")
		}

		// Continue as all information was written above with WriteHeader.
		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return errors.Wrapf(err, "error opening file for taring at: %s", path)
		}
		defer f.Close()

		bar := progress.DefaultBytes(info.Size(), info.Name())
		if _, err = io.Copy(io.MultiWriter(tw, bar), f); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	if err = tw.Close(); err != nil {
		log.Debugln("Error closing tar ball", err)
	}

	// Send the hash of all sent data, so our recipient can check the data.
	_, err = t.node.WriteBytes(s, se.Hash())
	if err != nil {
		return errors.Wrap(err, "error writing final hash to stream")
	}

	return t.node.WaitForEOF(s)
}

// relPath builds the path structure for the tar archive - this will be the structure as it is received.
func relPath(basePath string, baseIsDir bool, targetPath string) (string, error) {
	if baseIsDir {
		rel, err := filepath.Rel(basePath, targetPath)
		if err != nil {
			return "", err
		}
		return filepath.Clean(filepath.Join(filepath.Base(basePath), rel)), nil
	} else {
		return filepath.Base(basePath), nil
	}
}
