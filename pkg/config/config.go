package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/wrap"
)

const (
	Prefix = "pcp"
)

var (
	appIoutil wrap.Ioutiler = wrap.Ioutil{}
	appXdg    wrap.Xdger    = wrap.Xdg{}

	// relFilePath contains the path suffix that's appended to
	// an XDG compliant directory to find the settings file.
	relFilePath = filepath.Join(Prefix, "config.yaml")
)

type GlobalConfig struct {
	LogFile        string
	LogLevel       int
	LogAppend      bool
	DHT            bool
	MDNS           bool
	Homebrew       bool
	TelemetryHost  string
	TelemetryPort  int
	ConfigFile     string
	BootstrapPeers cli.StringSlice
}

func (c GlobalConfig) String() string {
	return fmt.Sprintf(
		"LogFile=%s LogLevel=%d LogAppend=%v DHT=%v MDNS=%v Homebrew=%v TelemetryHost=%s TelemetryPort=%d ConfigFile=%s",
		c.LogFile, c.LogLevel, c.LogAppend, c.DHT, c.MDNS, c.Homebrew, c.TelemetryHost, c.TelemetryPort, c.ConfigFile,
	)
}

var Global = GlobalConfig{}

func DefaultPath() (string, error) {
	return appXdg.ConfigFile(relFilePath)
}

type SendConfig struct {
	WordCount int
}

func (c SendConfig) String() string {
	return fmt.Sprintf("WordCount=%d", c.WordCount)
}

var Send = SendConfig{}

// Config contains general user settings and peer identity
// information. The configuration is split, so the identity
// information can easier be saved with more restrict
// access permissions as it contains the private Key.
type Config struct {
	Settings *Settings
}

// Save saves the peer settings and identity information
// to disk.
func (c *Config) Save() error {
	err := c.Settings.Save()
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

	c := &Config{
		Settings: settings,
	}

	return c, nil
}

func FillContext(c *cli.Context) (*cli.Context, error) {
	conf, err := LoadConfig()
	if err != nil {
		return c, err
	}
	c.Context = context.WithValue(c.Context, "ContextKey", conf)
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
