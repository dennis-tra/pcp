package node

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/crypt"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
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

	log.Debugf("Pake %s: %s: %s\n", peerID.String()[:16], PakeStepError, err)
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
		return fmt.Errorf("encrpyt challenge: %w", err)
	}

	if _, err = p.node.WriteBytes(s, challenge); err != nil {
		return fmt.Errorf("write bytes: %w", err)
	}

	return nil
}

// ReceiveVerifyProof reads proof data from the stream, decrypts it with the
// given key, that was derived via PAKE, and checks if it matches the remote
// public key.
func (p *PakeProtocol) ReceiveVerifyProof(s network.Stream, key []byte) error {
	response, err := p.node.ReadBytes(s)
	if err != nil {
		return fmt.Errorf("read bytes: %w", err)
	}

	dec, err := crypt.Decrypt(key, response)
	if err != nil {
		return fmt.Errorf("decrypt response: %w", err)
	}

	peerPubKey, err := p.node.Peerstore().PubKey(s.Conn().RemotePeer()).Raw()
	if err != nil {
		return fmt.Errorf("pet remote peer public key: %w", err)
	}

	if !bytes.Equal(dec, peerPubKey) {
		return fmt.Errorf("proof verification failed")
	}

	return nil
}
