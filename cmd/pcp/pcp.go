package main

import (
	"os"

	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/initialize"
	"github.com/dennis-tra/pcp/pkg/receive"
	"github.com/dennis-tra/pcp/pkg/send"
)

const (
	// Version of the PCP command line tool.
	Version = "0.0.1"
)

func main() {

	app := &cli.App{
		Name:                 "pcp",
		Usage:                "Peer Copy, a peer-to-peer data transfer tool.",
		Version:              Version,
		EnableBashCompletion: true,
		Commands: []*cli.Command{
			receive.Command,
			send.Command,
			initialize.Command,
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Load configuration from `FILE`",
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Infof("error: %v\n", err)
		os.Exit(1)
	}
}
