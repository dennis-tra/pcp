package mdns

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

type State struct {
	Stage Stage
	Err   error
}
