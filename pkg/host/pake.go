package host

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
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
	"github.com/dennis-tra/pcp/pkg/tui"
)

const ProtocolPake = "/pcp/pake/0.3.0"

var ErrAuthenticationFailed = fmt.Errorf("authentication failed")

type PakeProtocol struct {
	ctx    context.Context
	host   host.Host
	sender tea.Sender
	role   PakeRole

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
func NewPakeProtocol(ctx context.Context, host host.Host, sender tea.Sender, words []string) *PakeProtocol {
	return &PakeProtocol{
		ctx:    ctx,
		host:   host,
		sender: sender,
		pwKey:  []byte(strings.Join(words, "-")),
		states: map[peer.ID]*PakeState{},
	}
}

func (p *PakeProtocol) Init() tea.Cmd {
	return nil
}

func (p *PakeProtocol) Update(msg tea.Msg) (*PakeProtocol, tea.Cmd) {
	log.WithFields(logrus.Fields{
		"comp": "pake",
		"type": fmt.Sprintf("%T", msg),
	}).Tracef("handle message: %T\n", msg)

	switch msg := msg.(type) {
	case pakeOnKeyExchange:
		return p.handleOnKeyExchange(msg)
	case pakeMsg[PakeStep]:
		return p.handlePakeStepMsg(msg)
	case pakeMsg[error]:
		return p.handleError(msg)
	case pakeMsg[[]byte]:
		return p.handlePakeKey(msg)
	default:
		return p, nil
	}
}

func (p *PakeProtocol) PakeStateStr(peerID peer.ID) string {
	s, ok := p.states[peerID]
	if !ok {
		return "no key exchange started"
	}
	switch s.Step {
	case PakeStepStart:
		return "key exchange started"
	case PakeStepWaitingForKeyInformation:
		return "waiting for key information"
	case PakeStepCalculatingKeyInformation:
		return "calculating key information"
	case PakeStepSendingKeyInformation:
		return "sending key information"
	case PakeStepWaitingForFinalKeyInformation:
		return "waiting for final key information"
	case PakeStepExchangingSalt:
		return "exchanging salt"
	case PakeStepProvingAuthenticityToPeer:
		return "proving authenticity"
	case PakeStepVerifyingProofFromPeer:
		return "verifying proof from peer"
	case PakeStepWaitingForFinalConfirmation:
		return "waiting for final confirmation"
	case PakeStepPeerAuthenticated:
		return tui.Green.Render("Authenticated!")
	case PakeStepError:
		return tui.Red.Render("Authentication failed: " + s.Err.Error())
	default:
		return tui.Red.Render("unknown key exchange state")
	}
}

// IsAuthenticated checks if the given peer ID has successfully
// passed a password authenticated key exchange.
func (p *PakeProtocol) IsAuthenticated(peerID peer.ID) bool {
	as, ok := p.states[peerID]
	return ok && as.Step == PakeStepPeerAuthenticated
}

// GetSessionKey returns the session key that was obtained via
// the password authenticated key exchange (PAKE) protocol.
func (p *PakeProtocol) GetSessionKey(peerID peer.ID) []byte {
	state, ok := p.states[peerID]
	if !ok {
		return nil
	}

	return state.Key
}

type PakeRole string

const (
	PakeRoleSender   PakeRole = "SENDER"
	PakeRoleReceiver PakeRole = "RECEIVER"
)

func (p *PakeProtocol) RegisterKeyExchangeHandler(role PakeRole) {
	log.WithField("role", role).Infoln("Registering key exchange handler")
	p.role = role
	p.host.SetStreamHandler(ProtocolPake, func(s network.Stream) {
		p.sender.Send(pakeOnKeyExchange{stream: s})
	})
}

func (p *PakeProtocol) UnregisterKeyExchangeHandler() {
	log.Infoln("Unregistering key exchange handler")
	p.host.RemoveStreamHandler(ProtocolPake)
}

type pakeMsg[T any] struct {
	peerID  peer.ID
	payload T
	stream  network.Stream
}

type pakeOnKeyExchange struct {
	stream network.Stream
}

func (p *PakeProtocol) StartKeyExchange(ctx context.Context, remotePeer peer.ID) tea.Cmd {
	if exisiting, found := p.states[remotePeer]; found && exisiting.Step != PakeStepError {
		return nil
	}
	p.states[remotePeer] = &PakeState{Step: PakeStepStart}

	return func() tea.Msg {
		s, err := p.host.NewStream(ctx, remotePeer, ProtocolPake)
		pakeErrMsg := basePakeErrMsg(s, remotePeer)
		if err != nil {
			return pakeErrMsg(err)
		}

		defer s.Close()
		defer io.ResetOnShutdown(p.ctx, s)()

		// Authenticating peer...
		// initialize sender p ("0" indicates sender)
		P, err := pake.InitCurve(p.pwKey, 0, "siec")
		if err != nil {
			return pakeErrMsg(err)
		}

		// Sending key information...
		p.sendPakeMsg(s, PakeStepSendingKeyInformation)

		// Send Q init data
		if _, err = io.WriteBytes(s, P.Bytes()); err != nil {
			return pakeErrMsg(err)
		}

		// Waiting for key information...
		p.sendPakeMsg(s, PakeStepWaitingForKeyInformation)

		// Read calculated data from Q
		dat, err := io.ReadBytes(s)
		if err != nil {
			return pakeErrMsg(err)
		}

		// Calculating on key information...
		p.sendPakeMsg(s, PakeStepCalculatingKeyInformation)

		// Use calculated data from Q
		if err = P.Update(dat); err != nil {
			return pakeErrMsg(err)
		}

		// Sending key information...
		p.sendPakeMsg(s, PakeStepSendingKeyInformation)

		// Send Q calculated data
		if _, err = io.WriteBytes(s, P.Bytes()); err != nil {
			return pakeErrMsg(err)
		}

		// Extract calculated key
		skey, err := P.SessionKey()
		if err != nil {
			return pakeErrMsg(err)
		}

		p.sendPakeMsg(s, PakeStepExchangingSalt)

		// Reading salt
		salt, err := io.ReadBytes(s)
		if err != nil {
			return pakeErrMsg(err)
		}

		// derive key from PAKE session key + salt
		key, err := crypt.DeriveKey(skey, salt)
		if err != nil {
			return pakeErrMsg(fmt.Errorf("derive key from session key and salt: %w", err))
		}

		// Verifying proof from peer...
		p.sendPakeMsg(s, PakeStepVerifyingProofFromPeer)

		// Read and verify encryption proof from Q
		if err = p.receiveVerifyProof(s, key); err != nil {
			return pakeErrMsg(err)
		}

		// Proving authenticity to peer...
		p.sendPakeMsg(s, PakeStepProvingAuthenticityToPeer)

		// Send Q encryption proof
		if err = p.sendProof(s, key); err != nil {
			return pakeErrMsg(err)
		}

		// Waiting for confirmation from peer...
		p.sendPakeMsg(s, PakeStepWaitingForFinalConfirmation)

		// Read confirmation from P
		confirm, err := io.ReadBytes(s)
		if err != nil {
			return pakeErrMsg(err)
		}

		if string(confirm) != "ok" {
			return pakeErrMsg(fmt.Errorf("peer did not respond with ok"))
		}

		// Peer connected and authenticated!
		return pakeMsg[[]byte]{peerID: remotePeer, payload: key}
	}
}

func (p *PakeProtocol) exchangeKeys(s network.Stream) tea.Cmd {
	return func() tea.Msg {
		defer s.Close()
		defer io.ResetOnShutdown(p.ctx, s)()

		remotePeer := s.Conn().RemotePeer()

		pakeErrMsg := basePakeErrMsg(s, remotePeer)

		// keep track of stream
		p.sendPakeMsg(s, PakeStepStart)

		// initialize recipient Q ("1" indicates recipient)
		Q, err := pake.InitCurve(p.pwKey, 1, "siec")
		if err != nil {
			return pakeErrMsg(err)
		}

		// Waiting for key information...
		p.sendPakeMsg(s, PakeStepWaitingForKeyInformation)

		// Read init data from P
		dat, err := io.ReadBytes(s)
		if err != nil {
			return pakeErrMsg(err)
		}

		// Calculating on key information...
		p.sendPakeMsg(s, PakeStepCalculatingKeyInformation)

		// Use init data from P
		if err = Q.Update(dat); err != nil {
			return pakeErrMsg(err)
		}

		// Sending key information...
		p.sendPakeMsg(s, PakeStepSendingKeyInformation)

		// Send P calculated Data
		if _, err = io.WriteBytes(s, Q.Bytes()); err != nil {
			return pakeErrMsg(err)
		}

		// Waiting for final key information...
		p.sendPakeMsg(s, PakeStepWaitingForFinalKeyInformation)

		// Read calculated data from P
		dat, err = io.ReadBytes(s)
		if err != nil {
			return pakeErrMsg(err)
		}

		// Calculating on key information...
		p.sendPakeMsg(s, PakeStepCalculatingKeyInformation)

		// Use calculated data from P
		if err = Q.Update(dat); err != nil {
			return pakeErrMsg(err)
		}

		// Access session key
		skey, err := Q.SessionKey()
		if err != nil {
			return pakeErrMsg(err)
		}

		p.sendPakeMsg(s, PakeStepExchangingSalt)

		// Generating session key salt
		salt := make([]byte, 8)
		if _, err = rand.Read(salt); err != nil {
			return pakeErrMsg(fmt.Errorf("read randomness: %w", err))
		}

		// Sending salt
		if _, err = io.WriteBytes(s, salt); err != nil {
			return pakeErrMsg(err)
		}

		// Proving authenticity to peer...
		p.sendPakeMsg(s, PakeStepProvingAuthenticityToPeer)

		key, err := crypt.DeriveKey(skey, salt)
		if err != nil {
			return pakeErrMsg(err)
		}

		// Send P encryption proof
		if err = p.sendProof(s, key); err != nil {
			return pakeErrMsg(fmt.Errorf("send proof: %w", err))
		}

		// Verifying proof from peer...
		p.sendPakeMsg(s, PakeStepVerifyingProofFromPeer)

		// Read and verify encryption proof from P
		if err = p.receiveVerifyProof(s, key); err != nil {
			return pakeErrMsg(fmt.Errorf("receive verify proof: %w", err))
		}

		// Tell P the proof was verified and is okay
		if _, err = io.WriteBytes(s, []byte("ok")); err != nil {
			return pakeErrMsg(err)
		}

		// Wait for P to close the stream, so we know confirmation was received.
		if err = io.WaitForEOF(p.ctx, s); err != nil {
			log.Debugln("error waiting for EOF", err)
		}

		return pakeMsg[[]byte]{stream: s, peerID: remotePeer, payload: key}
	}
}

func (p *PakeProtocol) sendPakeMsg(stream network.Stream, s PakeStep) {
	msg := pakeMsg[PakeStep]{
		peerID:  stream.Conn().RemotePeer(),
		payload: s,
		stream:  stream,
	}
	p.sender.Send(msg)
}

func basePakeErrMsg(stream network.Stream, peerID peer.ID) func(err error) tea.Msg {
	return func(err error) tea.Msg {
		return pakeMsg[error]{stream: stream, peerID: peerID, payload: err}
	}
}

// sendProof takes the public key of our host and encrypts it with
// the PAKE-derived session key. The recipient can decrypt the key
// and verify that it matches.
func (p *PakeProtocol) sendProof(s network.Stream, key []byte) error {
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

// receiveVerifyProof reads proof data from the stream, decrypts it with the
// given key, that was derived via PAKE, and checks if it matches the remote
// public key.
func (p *PakeProtocol) receiveVerifyProof(s network.Stream, key []byte) error {
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
		return ErrAuthenticationFailed
	}

	return nil
}

func (p *PakeProtocol) handleOnKeyExchange(msg pakeOnKeyExchange) (*PakeProtocol, tea.Cmd) {
	remotePeer := msg.stream.Conn().RemotePeer()
	logEntry := log.WithField("remotePeer", remotePeer.ShortString())

	prevState, found := p.states[remotePeer]
	if found && prevState.Step == PakeStepError && errors.Is(prevState.Err, ErrAuthenticationFailed) {
		if err := msg.stream.Reset(); err != nil {
			logEntry.WithError(err).Warnln("Failed closing incoming pake stream")
		}
	} else {
		switch p.role {
		case PakeRoleSender:
			if found && prevState.stream != nil {
				if err := prevState.stream.Reset(); err != nil {
					logEntry.WithError(err).Warnln("Failed resetting outgoing pake stream")
				}
			}

			p.states[remotePeer] = &PakeState{
				Step:   PakeStepStart,
				stream: msg.stream,
			}
		case PakeRoleReceiver:
			if found {
				if err := msg.stream.Reset(); err != nil {
					logEntry.WithError(err).Warnln("Failed closing incoming pake stream")
				}
			} else {
				p.states[remotePeer] = &PakeState{
					Step:   PakeStepStart,
					stream: msg.stream,
				}
			}
		default:
			log.Fatalln("unexpected pake role:", p.role)
		}
	}

	return p, p.exchangeKeys(msg.stream)
}

func (p *PakeProtocol) handlePakeStepMsg(msg pakeMsg[PakeStep]) (*PakeProtocol, tea.Cmd) {
	state, ok := p.states[msg.peerID]
	if !ok {
		p.states[msg.peerID] = &PakeState{
			Step:   msg.payload,
			stream: msg.stream,
		}
		return p, nil
	}

	// either the existing state has no stream or the stream from the
	// message (if it exists) matches the existing stream id
	if state.stream == nil || (msg.stream != nil && msg.stream.ID() == state.stream.ID()) {
		switch state.Step {
		case PakeStepPeerAuthenticated:
		case PakeStepError:
			if errors.Is(state.Err, ErrAuthenticationFailed) {
				break
			}
			fallthrough
		default:
			p.states[msg.peerID] = &PakeState{
				Step:   msg.payload,
				stream: msg.stream,
			}
		}
	} else {
		msg.stream.Reset()
	}
	return p, nil
}

func (p *PakeProtocol) handleError(msg pakeMsg[error]) (*PakeProtocol, tea.Cmd) {
	log.WithError(msg.payload).WithField("remotePeer", msg.peerID.ShortString()).Debugf("Pake Error")

	prevState, found := p.states[msg.peerID]
	if found && prevState.stream != nil && prevState.stream.ID() != msg.stream.ID() {
		return p, nil
	}

	p.states[msg.peerID] = &PakeState{
		Step: PakeStepError,
		Err:  msg.payload,
	}

	return p, nil
}

func (p *PakeProtocol) handlePakeKey(msg pakeMsg[[]byte]) (*PakeProtocol, tea.Cmd) {
	p.states[msg.peerID] = &PakeState{
		Step:   PakeStepPeerAuthenticated,
		Key:    msg.payload,
		stream: msg.stream,
		Err:    nil,
	}

	var openStreams []network.Stream

	for pid, state := range p.states {
		if pid == msg.peerID || state.stream == nil {
			continue
		}
		openStreams = append(openStreams, state.stream)
	}

	return p, func() tea.Msg {
		for _, s := range openStreams {
			if err := s.Reset(); err != nil {
				log.WithError(err).WithField("peerID", s.Conn().RemotePeer().ShortString()).Warnln("Failed resetting stream")
			}
		}
		return nil
	}
}
