package mdns

type State string

const (
	StateIdle    State = "idle"
	StateStarted State = "started"
	StateError   State = "error"
	StateStopped State = "stopped"
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "StateIdle"
	case StateStarted:
		return "StateStarted"
	case StateError:
		return "StateError"
	case StateStopped:
		return "StateStopped"
	default:
		return "StateUnknown"
	}
}
