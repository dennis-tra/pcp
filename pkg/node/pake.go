package node

import (
	"bytes"
	"context"
	"crypto/elliptic"
	"fmt"
	"strings"
	"sync"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/schollz/pake/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/crypt"
)

// pattern: /protocol-name/request-or-response-message/version
const ProtocolPake = "/pcp/pake/0.2.0"

type PakeProtocol struct {
	node *Node

	// pwKey holds a scrypt derived key based on the users
	// given words that has the correct length to create
	// a new block cipher. It uses the key derivation
	// function (KDF) of scrypt. This is not the key that
	// is used to ultimately encrypt the communication.
	// This is the input key for PAKE.
	pwKey []byte

	// A map of peers that have successfully passed PAKE.
	// Peer.ID -> Session Key
	authedPeers sync.Map

	// Holds a key exchange handler that is called after
	// a successful key exchange.
	lk  sync.RWMutex
	keh KeyExchangeHandler
}

func NewPakeProtocol(node *Node, words []string) (*PakeProtocol, error) {
	pw := []byte(strings.Join(words, ""))
	key, err := crypt.DeriveKey(pw, node.pubKey)
	if err != nil {
		return nil, err
	}

	return &PakeProtocol{
		node:        node,
		pwKey:       key,
		authedPeers: sync.Map{},
		lk:          sync.RWMutex{},
	}, nil
}

// AddAuthenticatedPeer adds a peer ID and the session key that was
// obtained via the password authenticated peer exchange (PAKE) to
// a local peer store.
func (p *PakeProtocol) AddAuthenticatedPeer(peerID peer.ID, key []byte) {
	p.authedPeers.Store(peerID, key)
}

// IsAuthenticated checks if the given peer ID has successfully
// passed a password authenticated key exchange.
func (p *PakeProtocol) IsAuthenticated(peerID peer.ID) bool {
	_, found := p.authedPeers.Load(peerID)
	return found
}

// GetSessionKey returns the session key that was obtain via
// the password authenticated key exchange (PAKE) protocol.
func (p *PakeProtocol) GetSessionKey(peerID peer.ID) ([]byte, bool) {
	key, found := p.authedPeers.Load(peerID)
	if !found {
		return nil, false
	}
	sKey, ok := key.([]byte)
	if !ok {
		p.authedPeers.Delete(peerID)
		return nil, false
	}
	return sKey, true
}

type KeyExchangeHandler interface {
	HandleSuccessfulKeyExchange(peerID peer.ID)
}

func (p *PakeProtocol) RegisterKeyExchangeHandler(keh KeyExchangeHandler) {
	p.lk.Lock()
	defer p.lk.Unlock()
	p.keh = keh
	p.node.SetStreamHandler(ProtocolPake, p.onKeyExchange)
}

func (p *PakeProtocol) UnregisterKeyExchangeHandler() {
	p.lk.Lock()
	defer p.lk.Unlock()
	p.node.RemoveStreamHandler(ProtocolPake)
	p.keh = nil
}

func (p *PakeProtocol) onKeyExchange(s network.Stream) {
	defer s.Close()
	defer p.node.ResetOnShutdown(s)()

	log.Infor("Authenticating peer...")

	// pick an elliptic curve
	curve := elliptic.P521()

	// initialize recipient Q ("1" indicates recipient)
	Q, err := pake.Init(p.pwKey, 1, curve)
	if err != nil {
		log.Warningln(err)
		return
	}

	log.Infor("Waiting for key information...")
	// Read init data from P
	dat, err := p.node.ReadBytes(s)
	if err != nil {
		log.Warningln(err)
		return
	}

	log.Infor("Calculating on key information...")
	// Use init data from P
	if err = Q.Update(dat); err != nil {
		log.Warningln(err)
		return
	}

	log.Infor("Sending key information...")
	// Send P calculated Data
	if _, err = p.node.WriteBytes(s, Q.Bytes()); err != nil {
		log.Warningln(err)
		return
	}

	log.Infor("Waiting for final key information...")
	// Read calculated data from P
	dat, err = p.node.ReadBytes(s)
	if err != nil {
		log.Warningln(err)
		return
	}

	log.Infor("Calculating on key information...")
	// Use calculated data from P
	if err = Q.Update(dat); err != nil {
		log.Warningln(err)
		return
	}

	// Access session key
	key, err := Q.SessionKey()
	if err != nil {
		log.Warningln(err)
		return
	}

	log.Infor("Proofing authenticity to peer...")
	// Send P encryption proof
	if err := p.SendProof(s, key); err != nil {
		log.Warningln(err)
		return
	}

	log.Infor("Verifying proof from peer...")
	// Read and verify encryption proof from P
	if err := p.ReceiveVerifyProof(s, key); err != nil {
		log.Warningln(err)
		return
	}

	p.AddAuthenticatedPeer(s.Conn().RemotePeer(), key)

	// We're done reading data from P
	if err = s.CloseRead(); err != nil {
		log.Warningln("error closing pake write", err)
	}

	// Tell P the proof was verified and is okay
	if _, err = p.node.WriteBytes(s, []byte("ok")); err != nil {
		log.Warningln(err)
		return
	}

	// Wait for P to close the stream, so we know confirmation was received.
	if err = p.node.WaitForEOF(s); err != nil {
		log.Warningln("error waiting for EOF", err)
	}

	// We're done sending data over the stream.
	if err = s.Close(); err != nil {
		log.Warningln("error closing pake stream", err)
	}

	log.Infor("Peer connected and authenticated!\n")

	p.lk.RLock()
	defer p.lk.RUnlock()
	if p.keh == nil {
		return
	}

	go p.keh.HandleSuccessfulKeyExchange(s.Conn().RemotePeer())
}

func (p *PakeProtocol) StartKeyExchange(ctx context.Context, peerID peer.ID) ([]byte, error) {
	s, err := p.node.NewStream(ctx, peerID, ProtocolPake)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	log.Infor("Authenticating peer...")

	// pick an elliptic curve
	curve := elliptic.P521()

	// initialize sender p ("0" indicates sender)
	P, err := pake.Init(p.pwKey, 0, curve)
	if err != nil {
		return nil, err
	}

	log.Infor("Sending key information...")
	// Send Q init data
	if _, err = p.node.WriteBytes(s, P.Bytes()); err != nil {
		return nil, err
	}

	log.Infor("Waiting for key information...")
	// Read calculated data from Q
	dat, err := p.node.ReadBytes(s)
	if err != nil {
		return nil, err
	}

	log.Infor("Calculating on key information...")
	// Use calculated data from Q
	if err = P.Update(dat); err != nil {
		return nil, err
	}

	log.Infor("Sending key information...")
	// Send Q calculated data
	if _, err = p.node.WriteBytes(s, P.Bytes()); err != nil {
		return nil, err
	}

	// Extract calculated key
	key, err := P.SessionKey()
	if err != nil {
		return nil, err
	}

	log.Infor("Verifying proof from peer...")
	// Read and verify encryption proof from Q
	if err = p.ReceiveVerifyProof(s, key); err != nil {
		return nil, err
	}

	p.AddAuthenticatedPeer(s.Conn().RemotePeer(), key)

	log.Infor("Proofing authenticity to peer...")
	// Send Q encryption proof
	if err = p.SendProof(s, key); err != nil {
		return nil, err
	}

	// We're done sending data to Q
	if err = s.CloseWrite(); err != nil {
		log.Warningln("error closing pake write", err)
	}

	log.Infor("Waiting for confirmation from peer...")
	// Read confirmation from P
	confirm, err := p.node.ReadBytes(s)
	if err != nil {
		return nil, err
	}

	if string(confirm) != "ok" {
		return nil, fmt.Errorf("peer did not respond with ok")
	}

	log.Infor("Peer connected and authenticated!\n")
	return key, nil
}

// SendProof takes the public key of our node and encrypts it with
// the PAKE-derived session key. The recipient can decrypt the key
// and verify that it matches.
func (p *PakeProtocol) SendProof(s network.Stream, key []byte) error {
	challenge, err := crypt.Encrypt(key, p.node.pubKey)
	if err != nil {
		return err
	}

	if _, err := p.node.WriteBytes(s, challenge); err != nil {
		return err
	}

	return nil
}

// ReceiveVerifyProof reads proof data from the stream, decrypts it with the
// given key, that was derived via PAKE, and checks if it matches the remote
// public key.
func (p *PakeProtocol) ReceiveVerifyProof(s network.Stream, key []byte) error {
	response, err := p.node.ReadBytes(s)
	if err != nil {
		return err
	}

	dec, err := crypt.Decrypt(key, response)
	if err != nil {
		return err
	}

	peerPubKey, err := p.node.Peerstore().PubKey(s.Conn().RemotePeer()).Raw()
	if err != nil {
		return err
	}

	if !bytes.Equal(dec, peerPubKey) {
		return fmt.Errorf("proof verification failed")
	}

	return nil
}
