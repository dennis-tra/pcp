package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/receive"
	"github.com/dennis-tra/pcp/pkg/send"
)

// RawVersion is set via build flags.
var RawVersion = "dev"

func main() {
	app := &cli.App{
		Name: "peercp",
		Authors: []*cli.Author{
			{
				Name:  "Dennis Trautwein",
				Email: "peercp@dtrautwein.eu",
			},
		},
		Usage:                "Peer Copy, a peer-to-peer data transfer tool.",
		Version:              version(),
		EnableBashCompletion: true,
		Commands: []*cli.Command{
			receive.Command,
			send.Command,
		},
		Before: func(c *cli.Context) error {
			if c.IsSet("telemetry-host") || c.IsSet("telemetry-port") {
				go metricsListenAndServe(c.String("telemetry-host"), c.Int("telemetry-port"))
			}

			if c.Bool("debug") {
				log.SetLevel(log.DebugLevel)
			}
			return nil
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "enables debug log output",
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "shows more information about the local network",
			},
			&cli.BoolFlag{
				Name:    "dht",
				Aliases: []string{"remote"},
				Usage:   "only advertise via the DHT",
			},
			&cli.BoolFlag{
				Name:    "mdns",
				Aliases: []string{"local"},
				Usage:   "only advertise via multicast DNS",
			},
			&cli.BoolFlag{
				Name:   "homebrew",
				Usage:  "if set transfers a hard coded file with a hard coded word sequence",
				Hidden: true,
			},
			&cli.StringFlag{
				Name:    "telemetry-host",
				Usage:   "To which network address should the telemetry (prometheus, pprof) server bind",
				EnvVars: []string{"PEERCP_TELEMETRY_HOST", "PCP_TELEMETRY_HOST"},
				Value:   "localhost",
				Hidden:  true,
			},
			&cli.IntFlag{
				Name:    "telemetry-port",
				Usage:   "On which port should the telemetry (prometheus, pprof) server listen",
				EnvVars: []string{"PEERCP_TELEMETRY_PORT", "PCP_TELEMETRY_PORT"},
				Value:   0,
				Hidden:  true,
			},
		},
	}

	sigs := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(context.Background())

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	go func() {
		<-sigs
		log.Debugln("Stopping...")
		signal.Stop(sigs)
		cancel()
	}()

	err := app.RunContext(ctx, os.Args)
	if err != nil {
		log.Infof("error: %v\n", err)
		os.Exit(1)
	}
}

// version returns the version identifier for peercp in the following form: v0.5.0-d4aeaa2
func version() string {
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
					log.Warningln("couldn't parse vcs.revision:", err)
				}
			}
		}
	}

	if isDirty {
		shortCommit += "+dirty"
	}

	return fmt.Sprintf("v%s-%s", RawVersion, shortCommit)
}

func metricsListenAndServe(host string, port int) {
	addr := fmt.Sprintf("%s:%d", host, port)
	log.Debugln("Starting telemetry endpoint")
	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Warningln("Error serving prometheus:", err)
	}
}
