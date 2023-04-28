package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"sync"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/schollz/pake/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/crypt"
)

// pattern: /protocol-name/request-or-response-message/version
const ProtocolPake = "/pcp/pake/0.3.0"

type PakeProtocol struct {
	node *Node

	// pwKey holds the weak password of the PAKE protocol.
	// In our case it's the four words with dashes in between.
	pwKey []byte

	// A map of peers and their respective state in the
	// authentication process.
	statesLk sync.RWMutex
	states   map[peer.ID]*PakeState

	// Holds a key exchange handler that is called after
	// a successful key exchange.
	kehLk sync.RWMutex
	keh   KeyExchangeHandler
}

// NewPakeProtocol initializes a new PakeProtocol struct by deriving the weak session key
// from the given words. This is the basis for the strong session key that's derived from
// the PAKE protocol.
func NewPakeProtocol(node *Node, words []string) (PakeProtocol, error) {
	return PakeProtocol{
		node:   node,
		pwKey:  []byte(strings.Join(words, "-")),
		states: map[peer.ID]*PakeState{},
	}, nil
}

// setPakeStep sets the authentication state to the given step.
// If the peer isn't tracked yet, its state is created.
// It returns the previously stored step.
func (p *PakeProtocol) setPakeStep(peerID peer.ID, step PakeStep) PakeStep {
	p.statesLk.Lock()
	defer p.statesLk.Unlock()

	log.Debugf("Pake %s: %s\n", peerID.String()[:16], step)
	prevStep := PakeStepUnknown
	if _, ok := p.states[peerID]; ok {
		prevStep = p.states[peerID].Step
		p.states[peerID].Step = step
	} else {
		p.states[peerID] = &PakeState{Step: step}
	}

	return prevStep
}

// setPakeError sets the authentication state to PakeStepError with the given err value.
// If the peer isn't tracked yet, its state is created.
func (p *PakeProtocol) setPakeError(peerID peer.ID, err error) {
	p.statesLk.Lock()
	defer p.statesLk.Unlock()

	log.Debugf("%s: experienced error: %s", peerID.String(), err)
	p.states[peerID] = &PakeState{
		Step: PakeStepError,
		Err:  err,
	}
}

// PakeStates returns the authentication states for all peers that we have interacted with.
func (p *PakeProtocol) PakeStates() map[peer.ID]*PakeState {
	p.statesLk.RLock()
	defer p.statesLk.RUnlock()

	cpy := map[peer.ID]*PakeState{}
	for k, v := range p.states {
		cpy[k] = &PakeState{
			Step: v.Step,
			Err:  v.Err,
		}
		copy(cpy[k].Key, v.Key)
	}
	return cpy
}

// addSessionKey stores the given key for the given peer ID in the internal states map.
func (p *PakeProtocol) addSessionKey(peerID peer.ID, key []byte) {
	p.statesLk.Lock()
	defer p.statesLk.Unlock()

	log.Debugf("Storing session key for peer %s\n", peerID)
	if _, ok := p.states[peerID]; ok {
		p.states[peerID].Key = key
	} else {
		// unlikely to reach this code path
		p.states[peerID] = &PakeState{
			Step: PakeStepPeerAuthenticated,
			Key:  key,
		}
	}
}

// IsAuthenticated checks if the given peer ID has successfully
// passed a password authenticated key exchange.
func (p *PakeProtocol) IsAuthenticated(peerID peer.ID) bool {
	p.statesLk.RLock()
	defer p.statesLk.RUnlock()

	as, ok := p.states[peerID]
	return ok && as.Step == PakeStepPeerAuthenticated
}

// GetSessionKey returns the session key that was obtained via
// the password authenticated key exchange (PAKE) protocol.
func (p *PakeProtocol) GetSessionKey(peerID peer.ID) ([]byte, bool) {
	p.statesLk.RLock()
	defer p.statesLk.RUnlock()

	state, ok := p.states[peerID]
	if !ok {
		return nil, false
	}

	return state.Key, true
}

type KeyExchangeHandler interface {
	HandleSuccessfulKeyExchange(peerID peer.ID)
}

func (p *PakeProtocol) RegisterKeyExchangeHandler(keh KeyExchangeHandler) {
	log.Debugln("Registering key exchange handler")
	p.kehLk.Lock()
	defer p.kehLk.Unlock()

	p.keh = keh
	p.node.SetStreamHandler(ProtocolPake, p.onKeyExchange)
}

func (p *PakeProtocol) UnregisterKeyExchangeHandler() {
	log.Debugln("Unregistering key exchange handler")
	p.kehLk.Lock()
	defer p.kehLk.Unlock()

	p.node.RemoveStreamHandler(ProtocolPake)
	p.keh = nil
}

func (p *PakeProtocol) onKeyExchange(s network.Stream) {
	defer s.Close()
	defer p.node.ResetOnShutdown(s)()

	remotePeer := s.Conn().RemotePeer()

	// Authenticating peer...

	// In case the same remote peer "calls" onKeyExchange multiple times.
	// We only allow one concurrent key exchange per remote peer.
	prevStep := p.setPakeStep(remotePeer, PakeStepStart)
	switch prevStep {
	case PakeStepUnknown:
	case PakeStepError:
	default:
		log.Debugln("Rejecting key exchange request. Current step:", prevStep.String())
		return
	}

	// initialize recipient Q ("1" indicates recipient)
	Q, err := pake.InitCurve(p.pwKey, 1, "siec")
	if err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Waiting for key information...
	p.setPakeStep(remotePeer, PakeStepWaitingForKeyInformation)

	// Read init data from P
	dat, err := p.node.ReadBytes(s)
	if err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Calculating on key information...
	p.setPakeStep(remotePeer, PakeStepCalculatingKeyInformation)

	// Use init data from P
	if err = Q.Update(dat); err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Sending key information...
	p.setPakeStep(remotePeer, PakeStepSendingKeyInformation)

	// Send P calculated Data
	if _, err = p.node.WriteBytes(s, Q.Bytes()); err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Waiting for final key information...
	p.setPakeStep(remotePeer, PakeStepWaitingForFinalKeyInformation)

	// Read calculated data from P
	dat, err = p.node.ReadBytes(s)
	if err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Calculating on key information...
	p.setPakeStep(remotePeer, PakeStepCalculatingKeyInformation)

	// Use calculated data from P
	if err = Q.Update(dat); err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Access session key
	skey, err := Q.SessionKey()
	if err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	p.setPakeStep(remotePeer, PakeStepExchangingSalt)

	// Generating session key salt
	salt := make([]byte, 8)
	if _, err = rand.Read(salt); err != nil {
		p.setPakeError(remotePeer, fmt.Errorf("read randomness: %w", err))
		return
	}

	// Sending salt
	if _, err = p.node.WriteBytes(s, salt); err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Proving authenticity to peer...
	p.setPakeStep(remotePeer, PakeStepProvingAuthenticityToPeer)

	key, err := crypt.DeriveKey(skey, salt)
	if err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Send P encryption proof
	if err := p.SendProof(s, key); err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Verifying proof from peer...
	p.setPakeStep(remotePeer, PakeStepVerifyingProofFromPeer)

	// Read and verify encryption proof from P
	if err := p.ReceiveVerifyProof(s, key); err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	p.addSessionKey(remotePeer, key)
	p.setPakeStep(remotePeer, PakeStepPeerAuthenticated)

	// We're done reading data from P
	if err = s.CloseRead(); err != nil {
		log.Debugln("error closing pake write", err)
	}

	// Tell P the proof was verified and is okay
	if _, err = p.node.WriteBytes(s, []byte("ok")); err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Wait for P to close the stream, so we know confirmation was received.
	if err = p.node.WaitForEOF(s); err != nil {
		log.Debugln("error waiting for EOF", err)
	}

	// We're done sending data over the stream.
	if err = s.Close(); err != nil {
		log.Warningln("error closing pake stream", err)
	}

	p.kehLk.RLock()
	defer p.kehLk.RUnlock()
	if p.keh == nil {
		return
	}

	// needs to be in a go routine - if not, calls to (Un)register
	// stream handlers would deadlock
	go p.keh.HandleSuccessfulKeyExchange(remotePeer)
}

func (p *PakeProtocol) StartKeyExchange(ctx context.Context, peerID peer.ID) ([]byte, error) {
	s, err := p.node.NewStream(ctx, peerID, ProtocolPake)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	remotePeer := s.Conn().RemotePeer()

	// Authenticating peer...
	p.setPakeStep(remotePeer, PakeStepStart)

	// initialize sender p ("0" indicates sender)
	P, err := pake.InitCurve(p.pwKey, 0, "siec")
	if err != nil {
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	// Sending key information...
	p.setPakeStep(remotePeer, PakeStepSendingKeyInformation)

	// Send Q init data
	if _, err = p.node.WriteBytes(s, P.Bytes()); err != nil {
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	// Waiting for key information...
	p.setPakeStep(remotePeer, PakeStepWaitingForKeyInformation)

	// Read calculated data from Q
	dat, err := p.node.ReadBytes(s)
	if err != nil {
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	// Calculating on key information...
	p.setPakeStep(remotePeer, PakeStepCalculatingKeyInformation)

	// Use calculated data from Q
	if err = P.Update(dat); err != nil {
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	// Sending key information...
	p.setPakeStep(remotePeer, PakeStepSendingKeyInformation)

	// Send Q calculated data
	if _, err = p.node.WriteBytes(s, P.Bytes()); err != nil {
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	// Extract calculated key
	skey, err := P.SessionKey()
	if err != nil {
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	p.setPakeStep(remotePeer, PakeStepExchangingSalt)

	// Reading salt
	salt, err := p.node.ReadBytes(s)
	if err != nil {
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	// derive key from PAKE session key + salt
	key, err := crypt.DeriveKey(skey, salt)
	if err != nil {
		return nil, fmt.Errorf("derive key from session key and salt: %w", err)
	}

	// Verifying proof from peer...
	p.setPakeStep(remotePeer, PakeStepVerifyingProofFromPeer)

	// Read and verify encryption proof from Q
	if err = p.ReceiveVerifyProof(s, key); err != nil {
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	p.addSessionKey(s.Conn().RemotePeer(), key)

	// Proving authenticity to peer...
	p.setPakeStep(remotePeer, PakeStepProvingAuthenticityToPeer)

	// Send Q encryption proof
	if err = p.SendProof(s, key); err != nil {
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	// We're done sending data to Q
	if err = s.CloseWrite(); err != nil {
		log.Warningln("error closing pake write", err)
	}

	// Waiting for confirmation from peer...
	p.setPakeStep(remotePeer, PakeStepWaitingForFinalConfirmation)

	// Read confirmation from P
	confirm, err := p.node.ReadBytes(s)
	if err != nil {
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	if string(confirm) != "ok" {
		err = fmt.Errorf("peer did not respond with ok")
		p.setPakeError(remotePeer, err)
		return nil, err
	}

	// Peer connected and authenticated!
	p.setPakeStep(remotePeer, PakeStepPeerAuthenticated)

	return key, nil
}

// SendProof takes the public key of our node and encrypts it with
// the PAKE-derived session key. The recipient can decrypt the key
// and verify that it matches.
func (p *PakeProtocol) SendProof(s network.Stream, key []byte) error {
	pubKey, err := p.node.Peerstore().PubKey(p.node.ID()).Raw()
	if err != nil {
		return fmt.Errorf("get node pub key: %w", err)
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
