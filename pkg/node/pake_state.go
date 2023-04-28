package node

// PakeState holds information for a given peer that we are performing
// a password authenticated key exchange with. The struct stores the
// final derived strong session key as well as a potential error that has
// occurred during the exchange. Further, it keeps track of at which
// step of the key exchange we are with a given peer.
type PakeState struct {
	Step PakeStep
	Key  []byte
	Err  error
}

// PakeStep is an enum type that represents different steps in the
// password authenticated key exchange protocol.
type PakeStep uint8

const (
	PakeStepUnknown PakeStep = iota
	PakeStepStart
	PakeStepWaitingForKeyInformation
	PakeStepCalculatingKeyInformation
	PakeStepSendingKeyInformation
	PakeStepWaitingForFinalKeyInformation
	PakeStepExchangingSalt
	PakeStepProvingAuthenticityToPeer
	PakeStepVerifyingProofFromPeer
	PakeStepWaitingForFinalConfirmation
	PakeStepPeerAuthenticated
	PakeStepError
)

func (s PakeStep) String() string {
	switch s {
	case PakeStepStart:
		return "PakeStepStart"
	case PakeStepWaitingForKeyInformation:
		return "PakeStepWaitingForKeyInformation"
	case PakeStepCalculatingKeyInformation:
		return "PakeStepCalculatingKeyInformation"
	case PakeStepSendingKeyInformation:
		return "PakeStepSendingKeyInformation"
	case PakeStepWaitingForFinalKeyInformation:
		return "PakeStepWaitingForFinalKeyInformation"
	case PakeStepExchangingSalt:
		return "PakeStepExchangingSalt"
	case PakeStepProvingAuthenticityToPeer:
		return "PakeStepProvingAuthenticityToPeer"
	case PakeStepVerifyingProofFromPeer:
		return "PakeStepVerifyingProofFromPeer"
	case PakeStepWaitingForFinalConfirmation:
		return "PakeStepWaitingForFinalConfirmation"
	case PakeStepPeerAuthenticated:
		return "PakeStepPeerAuthenticated"
	case PakeStepError:
		return "PakeStepError"
	default:
		return "PakeStepUnknown"
	}
}
