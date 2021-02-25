package wrap

import (
	stdtime "time"
)

type Timer interface {
	AfterFunc(d stdtime.Duration, f func()) *stdtime.Timer
	NewTimer(d stdtime.Duration) *stdtime.Timer
	Now() stdtime.Time
	Sleep(d stdtime.Duration)
}

type Time struct{}

func (t Time) AfterFunc(d stdtime.Duration, f func()) *stdtime.Timer {
	return stdtime.AfterFunc(d, f)
}

func (t Time) Now() stdtime.Time {
	return stdtime.Now()
}

func (t Time) NewTimer(d stdtime.Duration) *stdtime.Timer {
	return stdtime.NewTimer(d)
}

func (t Time) Sleep(d stdtime.Duration) {
	stdtime.Sleep(d)
}
