package config

import (
	"fmt"
)

type SendConfig struct {
	WordCount int
}

func (c SendConfig) String() string {
	return fmt.Sprintf("WordCount=%d", c.WordCount)
}

func (c SendConfig) Validate() error {
	if c.WordCount < 3 && !Global.Homebrew {
		return fmt.Errorf("the number of words must not be less than 3")
	}

	return nil
}

var Send = SendConfig{
	WordCount: 4,
}
