package main

import (
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"reflect"
	"runtime"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"gopkg.in/yaml.v3"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"

	"github.com/dennis-tra/pcp/pkg/config"
)

// a handle to the file to log to in case the log-file flag is set
var logFile io.WriteCloser

func main() {
	configPath, err := config.DefaultPath()
	if err != nil {
		log.WithError(err).Warningln("Failed to initialize config file")
	}

	app := &cli.App{
		Name: "pcp",
		Authors: []*cli.Author{
			{
				Name:  "Dennis Trautwein",
				Email: "pcp@dtrautwein.eu",
			},
		},
		Usage:   "Peer Copy, a peer-to-peer data transfer tool.",
		Version: config.Global.Version,
		Commands: []*cli.Command{
			sendCmd,
			receiveCmd,
		},
		Before: beforeFunc,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				EnvVars:     []string{"PCP_CONFIG_FILE"},
				Usage:       "yaml config `FILE` name",
				Destination: &config.Global.ConfigFile,
				Value:       configPath,
			},
			altsrc.NewStringFlag(&cli.StringFlag{
				Name:        "log-file",
				Aliases:     []string{"log.file" /* config file key*/},
				EnvVars:     []string{"PCP_LOG_FILE"},
				Usage:       "writes log output to `FILE`",
				Destination: &config.Global.LogFile,
				Value:       config.Global.LogFile,
			}),
			altsrc.NewBoolFlag(&cli.BoolFlag{
				Name:        "log-append",
				Aliases:     []string{"log.append" /* config file key*/},
				EnvVars:     []string{"PCP_LOG_APPEND"},
				Usage:       "append to log output instead of overwriting",
				Destination: &config.Global.LogAppend,
				Value:       config.Global.LogAppend,
			}),
			altsrc.NewIntFlag(&cli.IntFlag{
				Name:        "log-level",
				Aliases:     []string{"log.level" /* config file key*/},
				EnvVars:     []string{"PCP_LOG_LEVEL"},
				Usage:       "a value from 0 (least verbose) to 6 (most verbose)",
				Destination: &config.Global.LogLevel,
				Value:       config.Global.LogLevel,
			}),
			altsrc.NewBoolFlag(&cli.BoolFlag{
				Name:        "dht",
				Aliases:     []string{"remote"},
				EnvVars:     []string{"PCP_DHT"},
				Usage:       "only advertise via the DHT",
				Destination: &config.Global.DHT,
				Value:       config.Global.DHT,
			}),
			altsrc.NewBoolFlag(&cli.BoolFlag{
				Name:        "mdns",
				Aliases:     []string{"local"},
				EnvVars:     []string{"PCP_MDNS"},
				Usage:       "only advertise via multicast DNS",
				Destination: &config.Global.MDNS,
				Value:       config.Global.MDNS,
			}),
			altsrc.NewBoolFlag(&cli.BoolFlag{
				Name:        "homebrew",
				Aliases:     []string{},
				EnvVars:     []string{"PCP_HOMEBREW"},
				Usage:       "if set transfers a hard coded file with a hard coded word sequence",
				Destination: &config.Global.Homebrew,
				Value:       config.Global.Homebrew,
				Hidden:      true,
			}),
			altsrc.NewStringFlag(&cli.StringFlag{
				Name:        "telemetry-host",
				Aliases:     []string{"telemetry.host" /* config file key*/},
				EnvVars:     []string{"PCP_TELEMETRY_HOST"},
				Usage:       "network address for prometheus and pprof to bind to. Set port to activate telemetry.",
				Destination: &config.Global.TelemetryHost,
				Value:       config.Global.TelemetryHost,
				Hidden:      true,
			}),
			altsrc.NewIntFlag(&cli.IntFlag{
				Name:        "telemetry-port",
				Aliases:     []string{"telemetry.port" /* config file key*/},
				EnvVars:     []string{"PCP_TELEMETRY_PORT"},
				Usage:       "port for prometheus and pprof to listen on. Set to activate telemetry.",
				Destination: &config.Global.TelemetryPort,
				Value:       config.Global.TelemetryPort,
				Hidden:      true,
			}),
			altsrc.NewStringSliceFlag(&cli.StringSliceFlag{
				Name:        "bootstrap-peers",
				Aliases:     []string{"bootstrapPeers" /* config file key*/},
				EnvVars:     []string{"PCP_BOOTSTRAP_PEERS"},
				Usage:       "port for prometheus and pprof to listen on. Set to activate telemetry.",
				DefaultText: "default IPFS",
				Value:       cli.NewStringSlice(config.BootstrapPeerMaddrStrings()...),
			}),
			altsrc.NewStringFlag(&cli.StringFlag{
				Name:        "protocol-id",
				Aliases:     []string{"protocol.id" /* config file key*/},
				EnvVars:     []string{"PCP_PROTOCOL_ID"},
				Usage:       "The protocol identifier to use when interacting with the DHT.",
				Destination: &config.Global.ProtocolID,
				Value:       config.Global.ProtocolID,
			}),
		},
		EnableBashCompletion: true,
	}

	err = app.Run(os.Args)

	if logFile != nil {
		log.Debugln("Closing log file.")
		if err := logFile.Close(); err != nil {
			fmt.Printf("error closing log file: %s\n", err)
		}
	}

	if err != nil {
		log.Errorf("error: %v\n", err)
		os.Exit(1)
	}
}

func beforeFunc(cCtx *cli.Context) error {
	if _, err := os.Stat(config.Global.ConfigFile); err == nil {
		yamlSrc := altsrc.NewYamlSourceFromFlagFunc("config")
		err := altsrc.InitInputSourceWithContext(cCtx.Command.Flags, yamlSrc)(cCtx)
		if err != nil {
			return fmt.Errorf("init yaml input src: %w", err)
		}
	}

	var bps []peer.AddrInfo
	for _, maddrStr := range cCtx.StringSlice("bootstrap-peers") {
		if maddrStr == "" {
			continue
		}

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

		bps = append(bps, *pi)
	}

	config.Global.BootstrapPeers = bps
	data, _ := yaml.Marshal(config.Global)
	fmt.Println(string(data))

	log.SetLevel(log.Level(config.Global.LogLevel))

	if config.Global.LogFile == "" {
		log.SetOutput(io.Discard)
	} else {

		flags := os.O_WRONLY | os.O_CREATE
		if config.Global.LogAppend {
			flags |= flags | os.O_APPEND
		}

		var err error
		logFile, err = os.OpenFile(config.Global.LogFile, flags, 0o644)
		if err != nil {
			return fmt.Errorf("open log file at %s: %w", config.Global.LogFile, err)
		}

		log.SetOutput(logFile)
	}

	if config.Global.TelemetryPort != 0 {
		go metricsListenAndServe(config.Global.TelemetryHost, config.Global.TelemetryPort)
	}

	if cCtx.IsSet("mdns") {
		config.Global.MDNS = cCtx.Bool("mdns")
		config.Global.DHT = false
		if cCtx.IsSet("dht") {
			config.Global.DHT = cCtx.Bool("dht")
		}
	} else if cCtx.IsSet("dht") {
		config.Global.DHT = cCtx.Bool("dht")
		config.Global.MDNS = false
		if cCtx.IsSet("mdns") {
			config.Global.MDNS = cCtx.Bool("mdns")
		}
	}

	return nil
}

func metricsListenAndServe(host string, port int) {
	addr := fmt.Sprintf("%s:%d", host, port)
	log.WithField("addr", addr).Debugln("Starting telemetry server")
	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.WithError(err).Warningln("Error serving telemetry")
	}
}

func filterFn(m tea.Model, msg tea.Msg) tea.Msg {
	batch, ok := msg.(tea.BatchMsg)
	if ok {
		names := []string{}
		for _, cmd := range batch {
			names = append(names, runtime.FuncForPC(reflect.ValueOf(cmd).Pointer()).Name())
		}
		log.WithField("size", len(batch)).WithField("batch", names).Tracef("tea filter: %T\n", msg)
	} else {
		log.Tracef("tea filter: %T\n", msg)
	}

	return msg
}
