package main

import (
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"

	"github.com/dennis-tra/pcp/pkg/config"
)

var (
	// RawVersion is set via build flags.
	RawVersion = "dev"

	// version is the fully qualified version string in the form: v0.5.0-d4aeaa2
	version = getVersion()

	bootstrapPeerMaddrStrings = getBootstrapPeerMaddrStrings()

	// a handle to the file to log to in case the log-file flag is set
	logFile io.WriteCloser
)

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
		Version: version,
		Commands: []*cli.Command{
			sendCmd,
			receiveCmd,
		},
		Before: beforeFunc,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				EnvVars:     []string{"P2P_CONFIG_FILE"},
				Usage:       "yaml config file name",
				Destination: &config.Global.ConfigFile,
				Value:       configPath,
			},
			altsrc.NewStringFlag(&cli.StringFlag{
				Name:        "log-file",
				Aliases:     []string{"log.file" /* config file key*/},
				EnvVars:     []string{"P2P_LOG_FILE"},
				Usage:       "writes log output to `FILE`",
				Destination: &config.Global.LogFile,
				Value:       "",
			}),
			altsrc.NewBoolFlag(&cli.BoolFlag{
				Name:        "log-append",
				Aliases:     []string{"log.append" /* config file key*/},
				EnvVars:     []string{"P2P_LOG_APPEND"},
				Usage:       "append to log output instead of overwriting",
				Destination: &config.Global.LogAppend,
				Value:       false,
			}),
			altsrc.NewIntFlag(&cli.IntFlag{
				Name:        "log-level",
				Aliases:     []string{"log.level" /* config file key*/},
				EnvVars:     []string{"P2P_LOG_LEVEL"},
				Usage:       "a value from 0 (least verbose) to 6 (most verbose)",
				Destination: &config.Global.LogLevel,
				Value:       int(log.InfoLevel),
			}),
			altsrc.NewBoolFlag(&cli.BoolFlag{
				Name:        "dht",
				Aliases:     []string{"remote"},
				EnvVars:     []string{"P2P_DHT"},
				Usage:       "only advertise via the DHT",
				Destination: &config.Global.DHT,
				Value:       true,
			}),
			altsrc.NewBoolFlag(&cli.BoolFlag{
				Name:        "mdns",
				Aliases:     []string{"local"},
				EnvVars:     []string{"P2P_MDNS"},
				Usage:       "only advertise via multicast DNS",
				Destination: &config.Global.MDNS,
				Value:       true,
			}),
			altsrc.NewBoolFlag(&cli.BoolFlag{
				Name:        "homebrew",
				Aliases:     []string{},
				EnvVars:     []string{"P2P_HOMEBREW"},
				Usage:       "if set transfers a hard coded file with a hard coded word sequence",
				Destination: &config.Global.Homebrew,
				Value:       false,
				Hidden:      true,
			}),
			altsrc.NewStringFlag(&cli.StringFlag{
				Name:        "telemetry-host",
				Aliases:     []string{"telemetry.host" /* config file key*/},
				EnvVars:     []string{"P2P_TELEMETRY_HOST"},
				Usage:       "network address for prometheus and pprof to bind to. Set port to activate telemetry.",
				Destination: &config.Global.TelemetryHost,
				Value:       "localhost",
				Hidden:      true,
			}),
			altsrc.NewIntFlag(&cli.IntFlag{
				Name:        "telemetry-port",
				Aliases:     []string{"telemetry.port" /* config file key*/},
				EnvVars:     []string{"PCP_TELEMETRY_PORT"},
				Usage:       "port for prometheus and pprof to listen on. Set to activate telemetry.",
				Destination: &config.Global.TelemetryPort,
				Value:       0,
				Hidden:      true,
			}),
			altsrc.NewStringSliceFlag(&cli.StringSliceFlag{
				Name:        "bootstrap-peers",
				Aliases:     []string{"bootstrapPeers" /* config file key*/},
				EnvVars:     []string{"PCP_BOOTSTRAP_PEERS"},
				Usage:       "port for prometheus and pprof to listen on. Set to activate telemetry.",
				Destination: &config.Global.BootstrapPeers,
				DefaultText: "default IPFS",
				Value:       cli.NewStringSlice(bootstrapPeerMaddrStrings...),
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

	config.Global.Version = cCtx.App.Version

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

func getVersion() string {
	var (
		err         error
		isDirty     bool
		shortCommit string
	)

	// read git commit sha and modified flag from go build information
	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, bs := range bi.Settings {
			switch bs.Key {
			case "vcs.revision":
				shortCommit = bs.Value
				if len(bs.Value) >= 7 {
					shortCommit = bs.Value[:7]
				}
			case "vcs.modified":
				isDirty, err = strconv.ParseBool(bs.Value)
				if err != nil {
					panic(fmt.Sprintf("couldn't parse vcs.revision: %s", err))
				}
			}
		}
	}

	if isDirty {
		shortCommit += "+dirty"
	}

	return fmt.Sprintf("v%s-%s", RawVersion, shortCommit)
}

func getBootstrapPeerMaddrStrings() []string {
	var maddrs []string
	for _, bp := range kaddht.GetDefaultBootstrapPeerAddrInfos() {
		for _, maddr := range bp.Addrs {
			comp, err := ma.NewComponent(ma.ProtocolWithCode(ma.P_P2P).Name, bp.ID.String())
			if err != nil {
				panic(err)
			}
			maddrs = append(maddrs, maddr.Encapsulate(comp).String())
		}
	}
	return maddrs
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
