package main

import (
	"fmt"
	"os"

	"github.com/dennis-tra/pcp/pkg/initialize"
	"github.com/dennis-tra/pcp/pkg/receive"
	"github.com/dennis-tra/pcp/pkg/send"
	"github.com/urfave/cli/v2"
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
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
