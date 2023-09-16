package dht

type State uint8

const (
	StateIdle = iota + 1
	StateBootstrapping
	StateBootstrapped
	StateActive
	StateStopping
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
	case StateActive:
		return "StateActive"
	case StateStopping:
		return "StateStopping"
	case StateStopped:
		return "StateStopped"
	case StateError:
		return "StateError"
	default:
		return "StateUnknown"
	}
}
