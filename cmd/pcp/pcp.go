package main

import (
	"log"
	"os"

	"github.com/dennis-tra/pcp/pkg/receive"
	"github.com/dennis-tra/pcp/pkg/send"
	"github.com/urfave/cli/v2"
)

const (
	// Version of the PCP command line tool
	Version = "0.0.1"
)

func main() {
	app := &cli.App{
		Name:                 "pcp",
		Usage:                "Peer Copy - ",
		Version:              Version,
		EnableBashCompletion: true,
		Commands: []*cli.Command{
			receive.Command,
			send.Command,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
