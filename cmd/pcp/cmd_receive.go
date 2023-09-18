package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/receive"
)

// receiveCmd contains the `receive` sub-command configuration.
var receiveCmd = &cli.Command{
	Name:      "receive",
	Usage:     "search for peers in your local network and the DHT",
	Aliases:   []string{"r", "get"},
	Action:    receiveAction,
	ArgsUsage: "[WORD-CODE]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:        "auto-accept",
			Aliases:     []string{"yes", "y", "receive.autoAccept" /* config file key*/},
			EnvVars:     []string{"PCP_AUTO_ACCEPT"},
			Usage:       "automatically accept the file transfer",
			Destination: &config.Receive.AutoAccept,
			Value:       config.Receive.AutoAccept,
		},
	},
	Description: `The receive subcommand starts searching for peers in your local 
network by sending out multicast DNS queries. These queries are
based on the current time and the first word of the given list. It
simultaneously queries the distributed hash table (DHT) with the
exact same parameters.

It is important to note that many networks restrict the use of
multicasting, which prevents mDNS from functioning. Notably,
multicast cannot be used in any sort of cloud, or shared infra-
structure environment. However it works well in most office, home,
or private infrastructure environments.

After it has found a potential peer it starts a password authen-
ticated key exchange (PAKE) with the remaining three words to
proof that the peer is in possession of the password. While this
is happening the tool still searches for other peers as the
currently connected one could fail the authentication procedure.

After the authentication was successful you need to confirm the
file transfer. The confirmation dialog shows the name and size of
the file.

The file will be saved to your current working directory overwriting
any files with the same name. If the transmission fails the file 
will contain the partial written bytes.`,
}

// receiveAction contains the logic for the `receive` subcommand of the pcp
// program.
func receiveAction(cCtx *cli.Context) error {
	program := tea.NewProgram(
		tea.WithoutSignalHandler(),
		tea.WithFilter(filterFn),
	)

	words := strings.Split(cCtx.Args().First(), "-")
	if len(words) < 3 {
		return fmt.Errorf("the number of words must not be less than 3")
	}

	state, err := receive.NewState(cCtx.Context, program, words)
	if err != nil {
		return fmt.Errorf("new send state: %w", err)
	}

	_, err = program.Run(state)

	return err
}
