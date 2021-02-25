package wrap

import (
	stdxdg "github.com/adrg/xdg"
)

type Xdger interface {
	ConfigFile(relPath string) (string, error)
}

type Xdg struct{}

func (a Xdg) ConfigFile(relPath string) (string, error) {
	return stdxdg.ConfigFile(relPath)
}
