package config

import (
	"fmt"
)

type ReceiveConfig struct {
	AutoAccept bool
}

func (c ReceiveConfig) String() string {
	return fmt.Sprintf("AutoAccept=%v", c.AutoAccept)
}

var Receive = ReceiveConfig{
	AutoAccept: false,
}
