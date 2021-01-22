package main

import (
	"fmt"
	"os"

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
		},
		ExitErrHandler: func(context *cli.Context, err error) {
			if err == nil {
				return
			}
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
