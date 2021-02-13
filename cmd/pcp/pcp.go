package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/receive"
	"github.com/dennis-tra/pcp/pkg/send"
)

var (
	// Version and build tag of the
	// PCP command line tool. This is
	// replaced on build via e.g.:
	// -ldflags "-X main.Version=${VERSION}"
	Version = "dev"
	Build   = "5f3759df" // quake
)

func main() {
	app := &cli.App{
		Name: "pcp",
		Authors: []*cli.Author{
			{
				Name:  "Dennis Trautwein",
				Email: "pcp@dtrautwein.eu",
			},
		},
		Usage:                "Peer Copy, a peer-to-peer data transfer tool.",
		Version:              fmt.Sprintf("%s+%s", Version, Build[:7]),
		EnableBashCompletion: true,
		Commands: []*cli.Command{
			receive.Command,
			send.Command,
		},
	}

	sigs := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(context.Background())

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	go func() {
		<-sigs
		log.Infoln("Stopping...")
		signal.Stop(sigs)
		cancel()
	}()

	err := app.RunContext(ctx, os.Args)
	if err != nil {
		log.Infof("error: %v\n", err)
		os.Exit(1)
	}
}
