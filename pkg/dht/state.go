package dht

type State uint8

const (
	StateIdle = iota + 1
	StateBootstrapping
	StateBootstrapped
	StateProviding
	StateLookup
	StateRetrying
	StateProvided
	StateStopped
	StateError
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "StateIdle"
	case StateBootstrapping:
		return "StateBootstrapping"
	case StateBootstrapped:
		return "StateBootstrapped"
	case StateProviding:
		return "StateProviding"
	case StateLookup:
		return "StateLookup"
	case StateRetrying:
		return "StateRetrying"
	case StateProvided:
		return "StateProvided"
	case StateStopped:
		return "StateStopped"
	case StateError:
		return "StateError"
	default:
		return "StateUnknown"
	}
}
