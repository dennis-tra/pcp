package config

import (
	"path/filepath"

	"github.com/dennis-tra/pcp/internal/wrap"
)

const (
	Prefix = "pcp"
)

var (
	appOs  wrap.Oser  = wrap.Os{}
	appXdg wrap.Xdger = wrap.Xdg{}

	// relFilePath contains the path suffix that's appended to
	// an XDG compliant directory to find the settings file.
	relFilePath = filepath.Join(Prefix, "config.yaml")
)

func DefaultPath() (string, error) {
	return appXdg.ConfigFile(relFilePath)
}
