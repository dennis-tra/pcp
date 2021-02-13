package config

import (
	"encoding/json"
	"os"
)

type Settings struct {
	Path   string `json:"-"`
	Exists bool   `json:"-"`
}

func LoadSettings() (*Settings, error) {
	path, err := appXdg.ConfigFile(settingsFile)
	if err != nil {
		return nil, err
	}

	settings := &Settings{Path: path}
	data, err := appIoutil.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &settings)
		if err != nil {
			return nil, err
		}
		settings.Exists = true
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	return settings, nil
}

func (s *Settings) Save() error {
	err := save(settingsFile, s, 0o744)
	if err == nil {
		s.Exists = true
	}
	return err
}
