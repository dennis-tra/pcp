package host

import "github.com/libp2p/go-libp2p/core/network"

// AuthState holds information for a given peer that we are performing
// a password authenticated key exchange with. The struct stores the
// final derived strong session key as well as a potential error that has
// occurred during the exchange. Further, it keeps track of at which
// step of the key exchange we are with a given peer.
type AuthState struct {
	Step AuthStep
	Key  []byte
	Err  error

	stream network.Stream
}

// AuthStep is an enum type that represents different steps in the
// password authenticated key exchange protocol.
type AuthStep uint8

const (
	AuthStepUnknown AuthStep = iota
	AuthStepStart
	AuthStepWaitingForKeyInformation
	AuthStepCalculatingKeyInformation
	AuthStepSendingKeyInformation
	AuthStepWaitingForFinalKeyInformation
	AuthStepExchangingSalt
	AuthStepProvingAuthenticityToPeer
	AuthStepVerifyingProofFromPeer
	AuthStepWaitingForFinalConfirmation
	AuthStepPeerAuthenticated
	AuthStepPeerSameRole
	AuthStepError
)

func (s AuthStep) String() string {
	switch s {
	case AuthStepStart:
		return "AuthStepStart"
	case AuthStepWaitingForKeyInformation:
		return "AuthStepWaitingForKeyInformation"
	case AuthStepCalculatingKeyInformation:
		return "AuthStepCalculatingKeyInformation"
	case AuthStepSendingKeyInformation:
		return "AuthStepSendingKeyInformation"
	case AuthStepWaitingForFinalKeyInformation:
		return "AuthStepWaitingForFinalKeyInformation"
	case AuthStepExchangingSalt:
		return "AuthStepExchangingSalt"
	case AuthStepProvingAuthenticityToPeer:
		return "AuthStepProvingAuthenticityToPeer"
	case AuthStepVerifyingProofFromPeer:
		return "AuthStepVerifyingProofFromPeer"
	case AuthStepWaitingForFinalConfirmation:
		return "AuthStepWaitingForFinalConfirmation"
	case AuthStepPeerSameRole:
		return "AuthStepPeerSameRole"
	case AuthStepPeerAuthenticated:
		return "AuthStepPeerAuthenticated"
	case AuthStepError:
		return "AuthStepError"
	default:
		return "AuthStepUnknown"
	}
}
