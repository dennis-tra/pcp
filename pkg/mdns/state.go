package mdns

type Stage string

const (
	StageIdle        Stage = "idle"
	StageAdvertising Stage = "advertising"
	StageError       Stage = "error"
	StageStopped     Stage = "stopped"
)

type State struct {
	Stage Stage
	Err   error
}
