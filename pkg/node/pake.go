package node

import (
	"bytes"
	"context"
	"crypto/elliptic"
	"fmt"
	"github.com/dennis-tra/pcp/pkg/crypt"
	"sync"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/schollz/pake"

	"github.com/dennis-tra/pcp/internal/log"
)

// pattern: /protocol-name/request-or-response-message/version
const ProtocolPake = "/pcp/pake/0.1.0"

type PakeProtocol struct {
	node *Node
	pw   []byte
}

func newPakeProtocol(node *Node, pw []byte) *PakeProtocol {
	return &PakeProtocol{node: node, pw: pw}
}

// PakeServerProtocol .
type PakeServerProtocol struct {
	*PakeProtocol
	lk  sync.RWMutex
	keh KeyExchangeHandler
}

type KeyExchangeHandler interface {
	HandleSuccessfulKeyExchange(peerID peer.ID)
}

func NewPakeServerProtocol(node *Node, pw []byte) *PakeServerProtocol {
	return &PakeServerProtocol{PakeProtocol: newPakeProtocol(node, pw), lk: sync.RWMutex{}}
}

func (p *PakeServerProtocol) RegisterKeyExchangeHandler(keh KeyExchangeHandler) {
	p.lk.Lock()
	defer p.lk.Unlock()
	p.keh = keh
	p.node.SetStreamHandler(ProtocolPake, p.onKeyExchange)
}

func (p *PakeServerProtocol) UnregisterKeyExchangeHandler() {
	p.lk.Lock()
	defer p.lk.Unlock()
	p.node.RemoveStreamHandler(ProtocolPake)
	p.keh = nil
}

func (p *PakeServerProtocol) onKeyExchange(s network.Stream) {
	defer s.Close()

	log.Infof("\rExchanging Keys...")

	// pick an elliptic curve
	curve := elliptic.P521()

	// initialize recipient Q ("1" indicates recipient)
	Q, err := pake.Init(p.pw, 1, curve)
	if err != nil {
		log.Warningln(err)
		return
	}

	// Read init data from P
	dat, err := p.node.ReadBytes(s)
	if err != nil {
		log.Warningln(err)
		return
	}

	// Use init data from P
	if err = Q.Update(dat); err != nil {
		log.Warningln(err)
		return
	}

	// Send P calculated Data
	if _, err = p.node.WriteBytes(s, Q.Bytes()); err != nil {
		log.Warningln(err)
		return
	}

	// Read calculated data from P
	dat, err = p.node.ReadBytes(s)
	if err != nil {
		log.Warningln(err)
		return
	}

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

	log.Infof("\rAuthenticating Peer...")

	// Send P encryption proof
	if err := p.SendProof(s, key); err != nil {
		log.Warningln(err)
		return
	}

	// Read and verify encryption proof from P
	if err := p.ReceiveVerifyProof(s, key); err != nil {
		log.Warningln(err)
		return
	}

	p.node.AddAuthenticatedPeer(s.Conn().RemotePeer())

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

	log.Infoln("Done!")

	p.lk.RLock()
	defer p.lk.RUnlock()
	if p.keh == nil {
		return
	}

	go p.keh.HandleSuccessfulKeyExchange(s.Conn().RemotePeer())
}

// PakeClientProtocol .
type PakeClientProtocol struct {
	*PakeProtocol
}

func NewPakeClientProtocol(node *Node, pw []byte) *PakeClientProtocol {
	return &PakeClientProtocol{newPakeProtocol(node, pw)}
}

func (p *PakeClientProtocol) StartKeyExchange(ctx context.Context, peerID peer.ID) ([]byte, error) {
	log.Infof("\rExchanging Keys...")
	s, err := p.node.NewStream(ctx, peerID, ProtocolPake)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	// pick an elliptic curve
	curve := elliptic.P521()

	// initialize sender p ("0" indicates sender)
	P, err := pake.Init(p.pw, 0, curve)
	if err != nil {
		return nil, err
	}

	// Send Q init data
	if _, err := p.node.WriteBytes(s, P.Bytes()); err != nil {
		return nil, err
	}

	// Read calculated data from Q
	dat, err := p.node.ReadBytes(s)
	if err != nil {
		return nil, err
	}

	// Use calculated data from Q
	if err = P.Update(dat); err != nil {
		return nil, err
	}

	// Send Q calculated data
	if _, err := p.node.WriteBytes(s, P.Bytes()); err != nil {
		return nil, err
	}

	// Extract calculated key
	key, err := P.SessionKey()
	if err != nil {
		return nil, err
	}

	log.Infof("\rAuthenticating Peer...")
	// Read and verify encryption proof from Q
	if err := p.ReceiveVerifyProof(s, key); err != nil {
		return nil, err
	}

	p.node.AddAuthenticatedPeer(s.Conn().RemotePeer())

	// Send Q encryption proof
	if err := p.SendProof(s, key); err != nil {
		return nil, err
	}

	// We're done sending data to Q
	if err = s.CloseWrite(); err != nil {
		log.Warningln("error closing pake write", err)
	}

	// Read confirmation from P
	confirm, err := p.node.ReadBytes(s)
	if err != nil {
		return nil, err
	}

	if string(confirm) != "ok" {
		return nil, fmt.Errorf("peer did not respond with ok")
	}

	log.Infoln("Done!")
	return key, nil
}

func (p *PakeProtocol) SendProof(s network.Stream, key []byte) error {
	pubKey, err := p.node.Peerstore().PubKey(p.node.ID()).Raw()
	if err != nil {
		return err
	}

	challenge, err := crypt.Encrypt(key, pubKey)
	if err != nil {
		return err
	}

	if _, err := p.node.WriteBytes(s, challenge); err != nil {
		return err
	}

	return nil
}

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
