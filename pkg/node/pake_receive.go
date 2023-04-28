package node

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/schollz/pake/v2"

	"github.com/dennis-tra/pcp/pkg/crypt"
)

func (p *PakeProtocol) StartKeyExchange(ctx context.Context, peerID peer.ID) ([]byte, error) {
	s, err := p.node.NewStream(ctx, peerID, ProtocolPake)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	defer p.node.ResetOnShutdown(s)()

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
