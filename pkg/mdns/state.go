package mdns

import "fmt"

type Stage string

const (
	StageIdle    Stage = "idle"
	StageRoaming Stage = "roaming"
	StageError   Stage = "error"
	StageStopped Stage = "stopped"
)

func (s Stage) IsTermination() bool {
	return s == StageStopped || s == StageError
}

func (s Stage) String() string {
	switch s {
	case StageIdle:
		return "StageIdle"
	case StageRoaming:
		return "StageRoaming"
	case StageError:
		return "StageError"
	case StageStopped:
		return "StageStopped"
	default:
		return "StageUnknown"
	}
}

type State struct {
	Stage Stage
	Err   error
}

func (s State) String() string {
	return fmt.Sprintf("stage=%s err=%v", s.Stage, s.Err)
}
