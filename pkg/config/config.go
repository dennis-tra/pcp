package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/dennis-tra/pcp/internal/app"
)

const (
	Prefix     = "pcp"
	ContextKey = "config"
)

var (
	// settingsFile contains the path suffix that's appended to
	// an XDG compliant directory to find the settings file.
	settingsFile = filepath.Join(Prefix, "settings.json")
	identityFile = filepath.Join(Prefix, "identity.json")
)

var (
	appIoutil app.Ioutiler = app.Ioutil{}
	appXdg    app.Xdger    = app.Xdg{}
)

// Config contains general user settings and peer identity
// information. The configuration is split, so the identity
// information can easier be saved with more restrict
// access permissions as it contains the private Key.
type Config struct {
	Settings *Settings
	Identity *Identity
}

// Save saves the peer settings and identity information
// to disk.
func (c *Config) Save() error {

	err := c.Settings.Save()
	if err != nil {
		return err
	}

	err = c.Identity.Save()
	if err != nil {
		return err
	}

	return nil
}

func LoadConfig() (*Config, error) {
	settings, err := LoadSettings()
	if err != nil {
		return nil, err
	}

	identity, err := LoadIdentity()
	if err != nil {
		return nil, err
	}

	c := &Config{
		Identity: identity,
		Settings: settings,
	}

	return c, nil
}

func save(relPath string, obj interface{}, perm os.FileMode) error {

	path, err := appXdg.ConfigFile(relPath)
	if err != nil {
		return err
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	err = appIoutil.WriteFile(path, data, perm)
	if err != nil {
		return err
	}

	return nil
}
