package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"

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
			case "vcs.modified":
				shortCommit = bs.Value
				if len(bs.Value) >= 7 {
					shortCommit = bs.Value[:7]
				}
			case "vcs.revision":
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
