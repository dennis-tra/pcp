package config

import (
	"encoding/json"
	"fmt"
	"io"

	kaddht "github.com/libp2p/go-libp2p-kad-dht"

	"github.com/libp2p/go-libp2p/core/peer"
	log "github.com/sirupsen/logrus"
)

// a handle to the file to log to in case the log-file flag is set
var logFile io.WriteCloser

var Global = GlobalConfig{
	Version:        getVersion(),
	LogFile:        "",
	LogLevel:       int(log.InfoLevel),
	LogAppend:      false,
	DHT:            true,
	MDNS:           true,
	Homebrew:       false,
	TelemetryHost:  "localhost",
	TelemetryPort:  0,
	ConfigFile:     "",
	ConnThreshold:  3,
	BootstrapPeers: make([]peer.AddrInfo, 0),
	ProtocolID:     string(kaddht.ProtocolDHT),
}

// GlobalConfig holds configuration that
type GlobalConfig struct {
	// Version holds the fully qualified version string in the form: v0.5.0-d4aeaa2
	Version string `json:"-"`

	// LogFile is the location where pcp logs should be written to
	LogFile string

	// LogLevel indicates which severity levels should be written to LogFile
	LogLevel int

	// LogAppend indicates whether logs should be appended to LogFile
	LogAppend bool

	// If DHT is true we try to discover our peer via the DHT
	DHT bool

	// if MDNS is true we try to discover our peer in the local network over mDNS
	MDNS bool

	// Homebrew is a special flag that, when true, starts pcp in a way that allows automatic testing of a file transfer
	Homebrew bool

	// TelemetryHost holds the value at which prometheus metrics could be extracted
	TelemetryHost string

	// TelemetryPort holds the port at which prometheus metrics can be extracted
	TelemetryPort int

	// ConfigFile is the file from which the configuration is read
	ConfigFile string

	// ConnThreshold means how many connections we need to start DHT discovery
	ConnThreshold int

	// BootstrapPeers holds a list of peers for bootstrapping
	BootstrapPeers []peer.AddrInfo

	// ProtocolID is the identifier to use when interacting with the DHT
	ProtocolID string
}

func (c *GlobalConfig) String() string {
	data, _ := json.Marshal(c)
	return string(data)
}

func (c *GlobalConfig) Validate() error {
	if !c.DHT && !c.MDNS {
		return fmt.Errorf("either the DHT or mDNS discovery mechanism need to be active")
	}

	if len(c.BootstrapPeers) < c.ConnThreshold {
		return fmt.Errorf("too few bootstrap peers configured (min %d)", c.ConnThreshold)
	}

	return nil
}

// BootstrapPeerMaddrStrings returns the string values of the bootstrap peer
// multiaddresses
func BootstrapPeerMaddrStrings() []string {
	maddrs := make([]string, len(kaddht.DefaultBootstrapPeers))
	for _, bp := range kaddht.DefaultBootstrapPeers {
		maddrs = append(maddrs, bp.String())
	}
	return maddrs
}
