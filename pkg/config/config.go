package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	log "github.com/sirupsen/logrus"
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
	Version        string `json:"-"`
	LogFile        string
	LogLevel       int
	LogAppend      bool
	DHT            bool
	MDNS           bool
	Homebrew       bool
	TelemetryHost  string
	TelemetryPort  int
	ConfigFile     string
	ConnThreshold  int
	BootstrapPeers cli.StringSlice
}

func (c *GlobalConfig) String() string {
	return fmt.Sprintf(
		"LogFile=%s LogLevel=%d LogAppend=%v DHT=%v MDNS=%v Homebrew=%v TelemetryHost=%s TelemetryPort=%d ConfigFile=%s",
		c.LogFile, c.LogLevel, c.LogAppend, c.DHT, c.MDNS, c.Homebrew, c.TelemetryHost, c.TelemetryPort, c.ConfigFile,
	)
}

func (c *GlobalConfig) BoostrapAddrInfos() []peer.AddrInfo {
	var peers []peer.AddrInfo
	for _, maddrStr := range c.BootstrapPeers.Value() {

		maddr, err := ma.NewMultiaddr(maddrStr)
		if err != nil {
			log.WithError(err).WithField("maddr", maddrStr).Warnln("Couldn't parse multiaddress")
			continue
		}

		pi, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.WithError(err).WithField("maddr", maddr).Warningln("Couldn't craft peer addr info")
			continue
		}

		peers = append(peers, *pi)
	}

	return peers
}

func (c *GlobalConfig) Validate() error {
	if !c.DHT && !c.MDNS {
		return fmt.Errorf("either the DHT or mDNS discovery mechanism need to be active")
	}

	if len(c.BoostrapAddrInfos()) < c.ConnThreshold {
		return fmt.Errorf("too few bootstrap peers configured (min %d)", c.ConnThreshold)
	}

	return nil
}

var Global = GlobalConfig{
	ConnThreshold: 3,
}

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

type ReceiveConfig struct {
	AutoAccept bool
}

func (c ReceiveConfig) String() string {
	return fmt.Sprintf("AutoAccept=%v", c.AutoAccept)
}

var Receive = ReceiveConfig{}

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
