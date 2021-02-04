package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/initialize"
	"github.com/dennis-tra/pcp/pkg/receive"
	"github.com/dennis-tra/pcp/pkg/send"
)

var (
	// Version of the PCP command line tool.
	Version = "compile-time"
	Build   = "compile-time"
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
