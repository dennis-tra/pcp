package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

const (
	Prefix = "pcp"
	ContextKey = "config"
)

var (
	// settingsFile contains the path suffix that's appended to
	// an XDG compliant directory to find the settings file.
	settingsFile = filepath.Join(Prefix, "settings.json")
	identityFile = filepath.Join(Prefix, "identity.json")
)

// Config contains general user settings and peer identity
// information. The configuration is split, so the identity
// information can easier be saved with more restrict
// access permissions as it contains the private Key.
type Config struct {
	Settings Settings
	Identity Identity
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
	path, err := xdg.ConfigFile(settingsFile)
	if err != nil {
		return nil, err
	}

	settings := Settings{Path: path}
	data, err := ioutil.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &settings)
		if err != nil {
			return nil, err
		}
		settings.Exists = true
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	path, err = xdg.ConfigFile(identityFile)

	identity := Identity{Path: path}
	data, err = ioutil.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &identity)
		if err != nil {
			return nil, err
		}
		identity.Exists = true
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	c := &Config{
		Identity: identity,
		Settings: settings,
	}

	return c, nil
}

func save(relPath string, obj interface{}, perm os.FileMode) error {

	path, err := xdg.ConfigFile(relPath)
	if err != nil {
		return err
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(path, data, perm)
	if err != nil {
		return err
	}

	return nil
}
