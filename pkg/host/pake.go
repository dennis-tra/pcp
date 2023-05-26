package host

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/schollz/pake/v2"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/crypt"
	"github.com/dennis-tra/pcp/pkg/io"
)

// pattern: /protocol-name/request-or-response-message/version
const ProtocolPake = "/pcp/pake/0.3.0"

type PakeProtocol struct {
	ctx     context.Context
	host    host.Host
	program *tea.Program

	// pwKey holds the weak password of the PAKE protocol.
	// In our case it's the four words with dashes in between.
	pwKey []byte

	// A map of peers and their respective state in the
	// authentication process.
	states map[peer.ID]*PakeState
}

// NewPakeProtocol initializes a new PakeProtocol struct by deriving the weak session key
// from the given words. This is the basis for the strong session key that's derived from
// the PAKE protocol.
func NewPakeProtocol(ctx context.Context, host host.Host, program *tea.Program, words []string) *PakeProtocol {
	return &PakeProtocol{
		ctx:     ctx,
		host:    host,
		program: program,
		pwKey:   []byte(strings.Join(words, "-")),
		states:  map[peer.ID]*PakeState{},
	}
}

func (p *PakeProtocol) Update(msg tea.Msg) (*PakeProtocol, tea.Cmd) {
	log.WithFields(logrus.Fields{
		"comp": "pake",
		"type": fmt.Sprintf("%T", msg),
	}).Tracef("handle message: %T\n", msg)

	switch msg := msg.(type) {
	case pakeMsg[error]:
		log.Debugf("Pake %s: %s: %s\n", msg.peerID.String()[:16], PakeStepError, msg.payload)
		p.states[msg.peerID] = &PakeState{
			Step: PakeStepError,
			Err:  msg.payload,
		}
	case pakeMsg[[]byte]:
		p.states[msg.peerID] = &PakeState{
			Step: PakeStepPeerAuthenticated,
			Key:  msg.payload,
		}
	case pakeMsg[PakeStep]:
		evt := msg
		if state, ok := p.states[evt.peerID]; ok {
			if state.Step != PakeStepPeerAuthenticated {
				p.states[evt.peerID].Step = evt.payload
			} else if evt.payload > PakeStepPeerAuthenticated {
				p.states[evt.peerID].Step = evt.payload
			}
		} else {
			p.states[evt.peerID] = &PakeState{Step: evt.payload}
		}
	case pakeOnKeyExchange:
		remotePeer := msg.Conn().RemotePeer()

		// Authenticating peer...

		// In case the same remote peer "calls" onKeyExchange multiple times.
		// We only allow one concurrent key exchange per remote peer.
		prevState, found := p.states[remotePeer]
		if found {
			switch prevState.Step {
			case PakeStepUnknown:
			case PakeStepError:
			default:
				log.Debugln("Rejecting key exchange request. Current step:", prevState.Step.String())
			}
		}

		p.states[remotePeer].Step = PakeStepStart
		return p, p.exchangeKeys(msg)
	}
	return p, nil
}

// IsAuthenticated checks if the given peer ID has successfully
// passed a password authenticated key exchange.
func (p *PakeProtocol) IsAuthenticated(peerID peer.ID) bool {
	as, ok := p.states[peerID]
	return ok && as.Step == PakeStepPeerAuthenticated
}

// GetSessionKey returns the session key that was obtained via
// the password authenticated key exchange (PAKE) protocol.
func (p *PakeProtocol) GetSessionKey(peerID peer.ID) ([]byte, bool) {
	state, ok := p.states[peerID]
	if !ok {
		return nil, false
	}

	return state.Key, true
}

func (p *PakeProtocol) RegisterKeyExchangeHandler() {
	log.Infoln("Registering key exchange handler")
	p.host.SetStreamHandler(ProtocolPake, func(s network.Stream) {
		p.program.Send(pakeOnKeyExchange(s))
	})
}

func (p *PakeProtocol) UnregisterKeyExchangeHandler() {
	log.Infoln("Unregistering key exchange handler")
	p.host.RemoveStreamHandler(ProtocolPake)
}

type pakeMsg[T any] struct {
	peerID  peer.ID
	payload T
}

type pakeOnKeyExchange network.Stream

func (p *PakeProtocol) exchangeKeys(s network.Stream) tea.Cmd {
	return func() tea.Msg {
		defer s.Close()
		defer io.ResetOnShutdown(p.ctx, s)()

		remotePeer := s.Conn().RemotePeer()

		// initialize recipient Q ("1" indicates recipient)
		Q, err := pake.InitCurve(p.pwKey, 1, "siec")
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Waiting for key information...
		p.sendPakeMsg(remotePeer, PakeStepWaitingForKeyInformation)

		// Read init data from P
		dat, err := io.ReadBytes(s)
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Calculating on key information...
		p.sendPakeMsg(remotePeer, PakeStepCalculatingKeyInformation)

		// Use init data from P
		if err = Q.Update(dat); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Sending key information...
		p.sendPakeMsg(remotePeer, PakeStepSendingKeyInformation)

		// Send P calculated Data
		if _, err = io.WriteBytes(s, Q.Bytes()); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Waiting for final key information...
		p.sendPakeMsg(remotePeer, PakeStepWaitingForFinalKeyInformation)

		// Read calculated data from P
		dat, err = io.ReadBytes(s)
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Calculating on key information...
		p.sendPakeMsg(remotePeer, PakeStepCalculatingKeyInformation)

		// Use calculated data from P
		if err = Q.Update(dat); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Access session key
		skey, err := Q.SessionKey()
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		p.sendPakeMsg(remotePeer, PakeStepExchangingSalt)

		// Generating session key salt
		salt := make([]byte, 8)
		if _, err = rand.Read(salt); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: fmt.Errorf("read randomness: %w", err)}
		}

		// Sending salt
		if _, err = io.WriteBytes(s, salt); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Proving authenticity to peer...
		p.sendPakeMsg(remotePeer, PakeStepProvingAuthenticityToPeer)

		key, err := crypt.DeriveKey(skey, salt)
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Send P encryption proof
		if err = p.SendProof(s, key); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: fmt.Errorf("send proof: %w", err)}
		}

		// Verifying proof from peer...
		p.sendPakeMsg(remotePeer, PakeStepVerifyingProofFromPeer)

		// Read and verify encryption proof from P
		if err = p.ReceiveVerifyProof(s, key); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: fmt.Errorf("receive verify proof: %w", err)}
		}

		// Tell P the proof was verified and is okay
		if _, err = io.WriteBytes(s, []byte("ok")); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Wait for P to close the stream, so we know confirmation was received.
		if err = io.WaitForEOF(p.ctx, s); err != nil {
			log.Debugln("error waiting for EOF", err)
		}

		return pakeMsg[[]byte]{peerID: remotePeer, payload: key}
	}
}

func (p *PakeProtocol) StartKeyExchange(ctx context.Context, remotePeer peer.ID) tea.Cmd {
	return func() tea.Msg {
		s, err := p.host.NewStream(ctx, remotePeer, ProtocolPake)
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		defer s.Close()
		defer io.ResetOnShutdown(p.ctx, s)()

		// Authenticating peer...
		p.sendPakeMsg(remotePeer, PakeStepStart)

		// initialize sender p ("0" indicates sender)
		P, err := pake.InitCurve(p.pwKey, 0, "siec")
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Sending key information...
		p.sendPakeMsg(remotePeer, PakeStepSendingKeyInformation)

		// Send Q init data
		if _, err = io.WriteBytes(s, P.Bytes()); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Waiting for key information...
		p.sendPakeMsg(remotePeer, PakeStepWaitingForKeyInformation)

		// Read calculated data from Q
		dat, err := io.ReadBytes(s)
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Calculating on key information...
		p.sendPakeMsg(remotePeer, PakeStepCalculatingKeyInformation)

		// Use calculated data from Q
		if err = P.Update(dat); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Sending key information...
		p.sendPakeMsg(remotePeer, PakeStepSendingKeyInformation)

		// Send Q calculated data
		if _, err = io.WriteBytes(s, P.Bytes()); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Extract calculated key
		skey, err := P.SessionKey()
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		p.sendPakeMsg(remotePeer, PakeStepExchangingSalt)

		// Reading salt
		salt, err := io.ReadBytes(s)
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// derive key from PAKE session key + salt
		key, err := crypt.DeriveKey(skey, salt)
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: fmt.Errorf("derive key from session key and salt: %w", err)}
		}

		// Verifying proof from peer...
		p.sendPakeMsg(remotePeer, PakeStepVerifyingProofFromPeer)

		// Read and verify encryption proof from Q
		if err = p.ReceiveVerifyProof(s, key); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Proving authenticity to peer...
		p.sendPakeMsg(remotePeer, PakeStepProvingAuthenticityToPeer)

		// Send Q encryption proof
		if err = p.SendProof(s, key); err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		// Waiting for confirmation from peer...
		p.sendPakeMsg(remotePeer, PakeStepWaitingForFinalConfirmation)

		// Read confirmation from P
		confirm, err := io.ReadBytes(s)
		if err != nil {
			return pakeMsg[error]{peerID: remotePeer, payload: err}
		}

		if string(confirm) != "ok" {
			return pakeMsg[error]{peerID: remotePeer, payload: fmt.Errorf("peer did not respond with ok")}
		}

		// Peer connected and authenticated!
		return pakeMsg[[]byte]{peerID: remotePeer, payload: key}
	}
}

func (p *PakeProtocol) sendPakeMsg(peerID peer.ID, s PakeStep) {
	msg := pakeMsg[PakeStep]{
		peerID:  peerID,
		payload: s,
	}
	p.program.Send(msg)
}

// SendProof takes the public key of our host and encrypts it with
// the PAKE-derived session key. The recipient can decrypt the key
// and verify that it matches.
func (p *PakeProtocol) SendProof(s network.Stream, key []byte) error {
	pubKey, err := p.host.Peerstore().PubKey(p.host.ID()).Raw()
	if err != nil {
		return fmt.Errorf("get node pub key: %w", err)
	}

	challenge, err := crypt.Encrypt(key, pubKey)
	if err != nil {
		return fmt.Errorf("encrpyt challenge: %w", err)
	}

	if _, err = io.WriteBytes(s, challenge); err != nil {
		return fmt.Errorf("write bytes: %w", err)
	}

	return nil
}

// ReceiveVerifyProof reads proof data from the stream, decrypts it with the
// given key, that was derived via PAKE, and checks if it matches the remote
// public key.
func (p *PakeProtocol) ReceiveVerifyProof(s network.Stream, key []byte) error {
	response, err := io.ReadBytes(s)
	if err != nil {
		return fmt.Errorf("read bytes: %w", err)
	}

	dec, err := crypt.Decrypt(key, response)
	if err != nil {
		return fmt.Errorf("decrypt response: %w", err)
	}

	peerPubKey, err := p.host.Peerstore().PubKey(s.Conn().RemotePeer()).Raw()
	if err != nil {
		return fmt.Errorf("pet remote peer public key: %w", err)
	}

	if !bytes.Equal(dec, peerPubKey) {
		return fmt.Errorf("proof verification failed")
	}

	return nil
}
