package config

type Settings struct {
	Path   string `json:"-"`
	Exists bool   `json:"-"`
}

func (s *Settings) Save() error {
	err := save(settingsFile, s, 0744)
	if err == nil {
		s.Exists = true
	}
	return err
}
