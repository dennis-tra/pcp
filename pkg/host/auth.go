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
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/schollz/pake/v2"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/crypt"
	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/io"
	"github.com/dennis-tra/pcp/pkg/tui"
)

const (
	ProtocolAuthRoleReceiver protocol.ID = "/pcp/auth_receiver/0.1.0"
	ProtocolAuthRoleSender   protocol.ID = "/pcp/auth_sender/0.1.0"
)

var ErrAuthenticationFailed = fmt.Errorf("authentication failed")

type ErrAuthenticationSameRole struct {
	remoteRole discovery.Role
}

func (e ErrAuthenticationSameRole) Error() string {
	switch e.remoteRole {
	case discovery.RoleReceiver:
		return "remote also wants to receive data"
	case discovery.RoleSender:
		return "remote also wants to send data"
	default:
		return "undefined type of pcp instance"
	}
}

type AuthProtocol struct {
	ctx     context.Context
	host    host.Host
	sender  tea.Sender
	role    discovery.Role
	inProt  protocol.ID // the protocol to listen on
	outProt protocol.ID // the protocol to reach out with

	// pwKey holds the weak password of the PAKE protocol.
	// In our case it's the four words with dashes in between.
	pwKey []byte

	// A map of peers and their respective state in the
	// authentication process.
	states map[peer.ID]*AuthState
}

// NewAuthProtocol initializes a new AuthProtocol struct by deriving the weak session key
// from the given words. This is the basis for the strong session key that's derived from
// the PAKE protocol.
func NewAuthProtocol(ctx context.Context, host host.Host, sender tea.Sender, role discovery.Role, words []string) *AuthProtocol {
	var (
		inProt  protocol.ID
		outProt protocol.ID
	)

	switch role {
	case discovery.RoleSender:
		inProt = ProtocolAuthRoleReceiver
		outProt = ProtocolAuthRoleSender
	case discovery.RoleReceiver:
		inProt = ProtocolAuthRoleSender
		outProt = ProtocolAuthRoleReceiver
	default:
		panic("unexpected role: " + role)
	}

	return &AuthProtocol{
		ctx:     ctx,
		host:    host,
		sender:  sender,
		role:    role,
		inProt:  inProt,
		outProt: outProt,
		pwKey:   []byte(strings.Join(words, "-")),
		states:  map[peer.ID]*AuthState{},
	}
}

func (a *AuthProtocol) Init() tea.Cmd {
	return nil
}

func (a *AuthProtocol) Update(msg tea.Msg) (*AuthProtocol, tea.Cmd) {
	log.WithFields(logrus.Fields{
		"comp": "auth",
		"type": fmt.Sprintf("%T", msg),
	}).Tracef("handle message: %T\n", msg)

	switch msg := msg.(type) {
	case authOnKeyExchange:
		return a.handleOnKeyExchange(msg)
	case authMsg[AuthStep]:
		return a.handleAuthStepMsg(msg)
	case authMsg[error]:
		return a.handleError(msg)
	case authMsg[[]byte]:
		return a.handlePakeKey(msg)
	default:
		return a, nil
	}
}

func (a *AuthProtocol) AuthStateStr(peerID peer.ID) string {
	s, ok := a.states[peerID]
	if !ok {
		return "no key exchange started"
	}

	switch s.Step {
	case AuthStepStart:
		return "key exchange started"
	case AuthStepWaitingForKeyInformation:
		return "waiting for key information"
	case AuthStepCalculatingKeyInformation:
		return "calculating key information"
	case AuthStepSendingKeyInformation:
		return "sending key information"
	case AuthStepWaitingForFinalKeyInformation:
		return "waiting for final key information"
	case AuthStepExchangingSalt:
		return "exchanging salt"
	case AuthStepProvingAuthenticityToPeer:
		return "proving authenticity"
	case AuthStepVerifyingProofFromPeer:
		return "verifying proof from peer"
	case AuthStepWaitingForFinalConfirmation:
		return "waiting for final confirmation"
	case AuthStepPeerAuthenticated:
		return tui.Green.Render("Authenticated!")
	case AuthStepPeerSameRole:
		return tui.Yellow.Render("also a " + string(a.role))
	case AuthStepError:
		return tui.Red.Render("Authentication failed: " + s.Err.Error())
	default:
		return tui.Red.Render("unknown key exchange state")
	}
}

// IsAuthenticated checks if the given peer ID has successfully
// passed a password authenticated key exchange.
func (a *AuthProtocol) IsAuthenticated(peerID peer.ID) bool {
	as, ok := a.states[peerID]
	return ok && as.Step == AuthStepPeerAuthenticated
}

// GetSessionKey returns the session key that was obtained via
// the password-authenticated key exchange (PAKE) protocol.
func (a *AuthProtocol) GetSessionKey(peerID peer.ID) []byte {
	state, ok := a.states[peerID]
	if !ok {
		return nil
	}

	return state.Key
}

func (a *AuthProtocol) RegisterKeyExchangeHandler() {
	log.WithField("role", a.role).Infoln("Registering key exchange handler")
	a.host.SetStreamHandler(a.inProt, func(s network.Stream) {
		a.sender.Send(authOnKeyExchange{stream: s})
	})
}

func (a *AuthProtocol) UnregisterKeyExchangeHandler() {
	log.Infoln("Unregistering key exchange handler")
	a.host.RemoveStreamHandler(a.inProt)
}

type authMsg[T any] struct {
	peerID  peer.ID
	payload T
	stream  network.Stream
}

type authOnKeyExchange struct {
	stream network.Stream
}

func (a *AuthProtocol) StartKeyExchange(ctx context.Context, remotePeer peer.ID) tea.Cmd {
	if existing, found := a.states[remotePeer]; found && existing.Step != AuthStepError {
		return nil
	}
	a.states[remotePeer] = &AuthState{Step: AuthStepStart}

	return func() tea.Msg {
		s, err := a.host.NewStream(ctx, remotePeer, a.outProt)
		if err != nil {
			return authMsg[error]{peerID: remotePeer, payload: err}
		}

		defer s.Close()
		defer io.ResetOnShutdown(a.ctx, s)()

		authErrMsg := baseAuthErrMsg(s, remotePeer)

		// Authenticating peer...
		// initialize sender p ("0" indicates sender)
		P, err := pake.InitCurve(a.pwKey, 0, "siec")
		if err != nil {
			return authErrMsg(err)
		}

		// Sending key information...
		a.sendAuthMsg(s, AuthStepSendingKeyInformation)

		// Send Q init data
		if _, err = io.WriteBytes(s, P.Bytes()); err != nil {
			return authErrMsg(err)
		}

		// Waiting for key information...
		a.sendAuthMsg(s, AuthStepWaitingForKeyInformation)

		// Read calculated data from Q
		dat, err := io.ReadBytes(s)
		if err != nil {
			return authErrMsg(err)
		}

		// Calculating on key information...
		a.sendAuthMsg(s, AuthStepCalculatingKeyInformation)

		// Use calculated data from Q
		if err = P.Update(dat); err != nil {
			return authErrMsg(err)
		}

		// Sending key information...
		a.sendAuthMsg(s, AuthStepSendingKeyInformation)

		// Send Q calculated data
		if _, err = io.WriteBytes(s, P.Bytes()); err != nil {
			return authErrMsg(err)
		}

		// Extract calculated key
		skey, err := P.SessionKey()
		if err != nil {
			return authErrMsg(err)
		}

		a.sendAuthMsg(s, AuthStepExchangingSalt)

		// Reading salt
		salt, err := io.ReadBytes(s)
		if err != nil {
			return authErrMsg(err)
		}

		// derive key from PAKE session key + salt
		key, err := crypt.DeriveKey(skey, salt)
		if err != nil {
			return authErrMsg(fmt.Errorf("derive key from session key and salt: %w", err))
		}

		// Verifying proof from peer...
		a.sendAuthMsg(s, AuthStepVerifyingProofFromPeer)

		// Read and verify encryption proof from Q
		if err = a.receiveVerifyProof(s, key); err != nil {
			return authErrMsg(err)
		}

		// Proving authenticity to peer...
		a.sendAuthMsg(s, AuthStepProvingAuthenticityToPeer)

		// Send Q encryption proof
		if err = a.sendProof(s, key); err != nil {
			return authErrMsg(err)
		}

		// Waiting for confirmation from peer...
		a.sendAuthMsg(s, AuthStepWaitingForFinalConfirmation)

		// Read confirmation from P
		confirm, err := io.ReadBytes(s)
		if err != nil {
			return authErrMsg(err)
		}

		if string(confirm) != "ok" {
			return authErrMsg(fmt.Errorf("peer did not respond with ok"))
		}

		// Peer connected and authenticated!
		return authMsg[[]byte]{peerID: remotePeer, payload: key}
	}
}

func (a *AuthProtocol) exchangeKeys(s network.Stream) tea.Cmd {
	return func() tea.Msg {
		defer s.Close()
		defer io.ResetOnShutdown(a.ctx, s)()

		remotePeer := s.Conn().RemotePeer()

		authErrMsg := baseAuthErrMsg(s, remotePeer)

		// keep track of stream
		a.sendAuthMsg(s, AuthStepStart)

		// initialize recipient Q ("1" indicates recipient)
		Q, err := pake.InitCurve(a.pwKey, 1, "siec")
		if err != nil {
			return authErrMsg(err)
		}

		// Waiting for key information...
		a.sendAuthMsg(s, AuthStepWaitingForKeyInformation)

		// Read init data from P
		dat, err := io.ReadBytes(s)
		if err != nil {
			return authErrMsg(err)
		}

		// Calculating on key information...
		a.sendAuthMsg(s, AuthStepCalculatingKeyInformation)

		// Use init data from P
		if err = Q.Update(dat); err != nil {
			return authErrMsg(err)
		}

		// Sending key information...
		a.sendAuthMsg(s, AuthStepSendingKeyInformation)

		// Send P calculated Data
		if _, err = io.WriteBytes(s, Q.Bytes()); err != nil {
			return authErrMsg(err)
		}

		// Waiting for final key information...
		a.sendAuthMsg(s, AuthStepWaitingForFinalKeyInformation)

		// Read calculated data from P
		dat, err = io.ReadBytes(s)
		if err != nil {
			return authErrMsg(err)
		}

		// Calculating on key information...
		a.sendAuthMsg(s, AuthStepCalculatingKeyInformation)

		// Use calculated data from P
		if err = Q.Update(dat); err != nil {
			return authErrMsg(err)
		}

		// Access session key
		skey, err := Q.SessionKey()
		if err != nil {
			return authErrMsg(err)
		}

		a.sendAuthMsg(s, AuthStepExchangingSalt)

		// Generating session key salt
		salt := make([]byte, 8)
		if _, err = rand.Read(salt); err != nil {
			return authErrMsg(fmt.Errorf("read randomness: %w", err))
		}

		// Sending salt
		if _, err = io.WriteBytes(s, salt); err != nil {
			return authErrMsg(err)
		}

		// Proving authenticity to peer...
		a.sendAuthMsg(s, AuthStepProvingAuthenticityToPeer)

		key, err := crypt.DeriveKey(skey, salt)
		if err != nil {
			return authErrMsg(err)
		}

		// Send P encryption proof
		if err = a.sendProof(s, key); err != nil {
			return authErrMsg(fmt.Errorf("send proof: %w", err))
		}

		// Verifying proof from peer...
		a.sendAuthMsg(s, AuthStepVerifyingProofFromPeer)

		// Read and verify encryption proof from P
		if err = a.receiveVerifyProof(s, key); err != nil {
			return authErrMsg(fmt.Errorf("receive verify proof: %w", err))
		}

		// Tell P the proof was verified and is okay
		if _, err = io.WriteBytes(s, []byte("ok")); err != nil {
			return authErrMsg(err)
		}

		// Wait for P to close the stream, so we know confirmation was received.
		if err = io.WaitForEOF(a.ctx, s); err != nil {
			log.Debugln("error waiting for EOF", err)
		}

		return authMsg[[]byte]{stream: s, peerID: remotePeer, payload: key}
	}
}

func (a *AuthProtocol) sendAuthMsg(stream network.Stream, s AuthStep) {
	msg := authMsg[AuthStep]{
		peerID:  stream.Conn().RemotePeer(),
		payload: s,
		stream:  stream,
	}
	a.sender.Send(msg)
}

func baseAuthErrMsg(stream network.Stream, peerID peer.ID) func(err error) tea.Msg {
	return func(err error) tea.Msg {
		return authMsg[error]{stream: stream, peerID: peerID, payload: err}
	}
}

// sendProof takes the public key of our host and encrypts it with
// the PAKE-derived session key. The recipient can decrypt the key
// and verify that it matches.
func (a *AuthProtocol) sendProof(s network.Stream, key []byte) error {
	pubKey, err := a.host.Peerstore().PubKey(a.host.ID()).Raw()
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
func (a *AuthProtocol) receiveVerifyProof(s network.Stream, key []byte) error {
	response, err := io.ReadBytes(s)
	if err != nil {
		return fmt.Errorf("read bytes: %w", err)
	}

	dec, err := crypt.Decrypt(key, response)
	if err != nil {
		return fmt.Errorf("decrypt response: %w", err)
	}

	peerPubKey, err := a.host.Peerstore().PubKey(s.Conn().RemotePeer()).Raw()
	if err != nil {
		return fmt.Errorf("pet remote peer public key: %w", err)
	}

	if !bytes.Equal(dec, peerPubKey) {
		return ErrAuthenticationFailed
	}

	return nil
}

func (a *AuthProtocol) handleOnKeyExchange(msg authOnKeyExchange) (*AuthProtocol, tea.Cmd) {
	remotePeer := msg.stream.Conn().RemotePeer()
	logEntry := log.WithField("remotePeer", remotePeer.ShortString())

	prevState, found := a.states[remotePeer]
	if found && prevState.Step == AuthStepError && errors.Is(prevState.Err, ErrAuthenticationFailed) {
		if err := msg.stream.Reset(); err != nil {
			logEntry.WithError(err).Warnln("Failed closing incoming auth stream")
		}
	} else {
		switch a.role {
		case discovery.RoleSender:
			if found && prevState.stream != nil {
				if err := prevState.stream.Reset(); err != nil {
					logEntry.WithError(err).Warnln("Failed resetting outgoing auth stream")
				}
			}

			a.states[remotePeer] = &AuthState{
				Step:   AuthStepStart,
				stream: msg.stream,
			}
		case discovery.RoleReceiver:
			if found {
				if err := msg.stream.Reset(); err != nil {
					logEntry.WithError(err).Warnln("Failed closing incoming auth stream")
				}
			} else {
				a.states[remotePeer] = &AuthState{
					Step:   AuthStepStart,
					stream: msg.stream,
				}
			}
		default:
			log.Fatalln("unexpected auth role:", a.role)
		}
	}

	return a, a.exchangeKeys(msg.stream)
}

func (a *AuthProtocol) handleAuthStepMsg(msg authMsg[AuthStep]) (*AuthProtocol, tea.Cmd) {
	state, ok := a.states[msg.peerID]
	if !ok {
		a.states[msg.peerID] = &AuthState{
			Step:   msg.payload,
			stream: msg.stream,
		}
		return a, nil
	}

	// either the existing state has no stream or the stream from the
	// message (if it exists) matches the existing stream id
	if state.stream == nil || (msg.stream != nil && msg.stream.ID() == state.stream.ID()) {
		switch state.Step {
		case AuthStepPeerAuthenticated:
		case AuthStepError:
			if errors.Is(state.Err, ErrAuthenticationFailed) {
				break
			}
			fallthrough
		default:
			a.states[msg.peerID] = &AuthState{
				Step:   msg.payload,
				stream: msg.stream,
			}
		}
	} else {
		msg.stream.Reset()
	}
	return a, nil
}

func (a *AuthProtocol) handleError(msg authMsg[error]) (*AuthProtocol, tea.Cmd) {
	log.WithError(msg.payload).WithField("remotePeer", msg.peerID.ShortString()).Debugf("Auth Error")

	prevState, found := a.states[msg.peerID]
	if found && prevState.stream != nil && prevState.stream.ID() != msg.stream.ID() {
		return a, nil
	}

	isProtocolErr := strings.Contains(msg.payload.Error(), "protocols not supported")
	protocols, err := a.host.Peerstore().SupportsProtocols(msg.peerID, a.inProt)
	if err == nil && isProtocolErr && len(protocols) != 0 {
		a.states[msg.peerID] = &AuthState{
			Step: AuthStepPeerSameRole,
			Err:  msg.payload,
		}
	} else {
		a.states[msg.peerID] = &AuthState{
			Step: AuthStepError,
			Err:  msg.payload,
		}
	}

	return a, nil
}

func (a *AuthProtocol) handlePakeKey(msg authMsg[[]byte]) (*AuthProtocol, tea.Cmd) {
	a.states[msg.peerID] = &AuthState{
		Step:   AuthStepPeerAuthenticated,
		Key:    msg.payload,
		stream: msg.stream,
		Err:    nil,
	}

	var openStreams []network.Stream

	for pid, state := range a.states {
		if pid == msg.peerID || state.stream == nil {
			continue
		}
		openStreams = append(openStreams, state.stream)
	}

	return a, func() tea.Msg {
		for _, s := range openStreams {
			if err := s.Reset(); err != nil {
				log.WithError(err).WithField("peerID", s.Conn().RemotePeer().ShortString()).Warnln("Failed resetting stream")
			}
		}
		return nil
	}
}
