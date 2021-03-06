package node

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/dennis-tra/pcp/pkg/crypt"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/dennis-tra/pcp/internal/log"
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
	defer t.node.ResetOnShutdown(s)()

	// Get PAKE session key for stream decryption
	sKey, found := t.node.GetSessionKey(s.Conn().RemotePeer())
	if !found {
		log.Warningln("Received transfer from unauthenticated peer")
		return
	}

	// Read initialization vector from stream. This is sent first from our peer.
	iv, err := t.node.ReadBytes(s)
	if err != nil {
		log.Warningln("Could not read stream initialization vector", err)
		return
	}

	t.lk.RLock()
	defer func() {
		if err = s.Close(); err != nil {
			log.Warningln(err)
		}
		t.lk.RUnlock()
	}()

	// Only read as much as we expect to avoid stuffing.
	lr := io.LimitReader(s, t.th.GetLimit())

	// Decrypt the stream
	sd, err := crypt.NewStreamDecrypter(sKey, iv, lr)
	if err != nil {
		log.Warningln("Could not instantiate stream decrypter", err)
		return
	}

	t.th.HandleTransfer(sd)

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
func (t *TransferProtocol) Transfer(ctx context.Context, peerID peer.ID, progress io.Writer, src io.Reader) error {
	// Open a new stream to our peer.
	s, err := t.node.NewStream(ctx, peerID, ProtocolTransfer)
	if err != nil {
		return err
	}
	defer s.Close()
	defer t.node.ResetOnShutdown(s)()

	// Get PAKE session key for stream encryption
	sKey, found := t.node.GetSessionKey(peerID)
	if !found {
		return fmt.Errorf("session key not found to encrypt data transfer")
	}

	// Initialize new stream encrypter
	se, err := crypt.NewStreamEncrypter(sKey, src)
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

	// The actual file transfer.
	_, err = io.Copy(io.MultiWriter(s, progress), se)
	if err != nil {
		return err
	}

	// Send the hash of all sent data, so our recipient can check the data.
	_, err = t.node.WriteBytes(s, se.Hash())
	if err != nil {
		return err
	}

	return t.node.WaitForEOF(s)
}
