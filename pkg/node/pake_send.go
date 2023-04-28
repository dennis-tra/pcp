package node

import (
	"crypto/rand"
	"fmt"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/schollz/pake/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/crypt"
)

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
	if err = p.SendProof(s, key); err != nil {
		p.setPakeError(remotePeer, fmt.Errorf("send proof: %w", err))
		return
	}

	// Verifying proof from peer...
	p.setPakeStep(remotePeer, PakeStepVerifyingProofFromPeer)

	// Read and verify encryption proof from P
	if err = p.ReceiveVerifyProof(s, key); err != nil {
		p.setPakeError(remotePeer, fmt.Errorf("receive verify proof: %w", err))
		return
	}

	p.addSessionKey(remotePeer, key)
	p.setPakeStep(remotePeer, PakeStepPeerAuthenticated)

	// Tell P the proof was verified and is okay
	if _, err = p.node.WriteBytes(s, []byte("ok")); err != nil {
		p.setPakeError(remotePeer, err)
		return
	}

	// Wait for P to close the stream, so we know confirmation was received.
	if err = p.node.WaitForEOF(s); err != nil {
		log.Debugln("error waiting for EOF", err)
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
