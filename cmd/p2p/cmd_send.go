package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dennis-tra/pcp/pkg/send"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/pkg/config"
)

// sendCmd holds the `send` subcommand configuration.
var sendCmd = &cli.Command{
	Name:    "send",
	Usage:   "make the given file available to your peer",
	Aliases: []string{"s"},
	Action:  Action,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:        "w",
			Aliases:     []string{"word-count", "send.wordCount" /* config file key*/},
			EnvVars:     []string{"PCP_WORD_COUNT"},
			Usage:       "the number of random words to use (min 3)",
			Destination: &config.Send.WordCount,
			Value:       4,
			Action: func(cCtx *cli.Context, wordCount int) error {
				log.Debugln("Validating given word count:", config.Send.WordCount)
				if wordCount < 3 && !config.Global.Homebrew {
					return fmt.Errorf("the number of words must not be less than 3")
				}
				return nil
			},
		},
	},
	ArgsUsage: `FILE`,
	Description: `
The send subcommand generates four random words based on the first
bytes of a newly generated peer identity. The first word and the
current time are used to generate an identifier that is broadcasted
in your local network via mDNS and provided through the distributed
hash table of the IPFS network.

After a peer attempts to connect it starts a password authen-
ticated key exchange (PAKE) with the remaining three words to
proof that the peer is in possession of the password. While this
is happening the tool still searches for other peers as the
currently connected one could fail the authentication procedure.

After the authentication was successful and the peer confirmed
the file transfer the transmission is started.
`,
}

// Action contains the logic for the `send` subcommand of the pcp program. It is
// mainly responsible for input parsing and service initialisation.
func Action(cCtx *cli.Context) error {
	state, err := send.NewState(cCtx.Context, cCtx.Args().First())
	if err != nil {
		return fmt.Errorf("new send state: %w", err)
	}

	teaFilter := tea.WithFilter(func(m tea.Model, msg tea.Msg) tea.Msg {
		log.Tracef("tea filter: %T\n", msg)
		return msg
	})

	p := tea.NewProgram(state, tea.WithoutSignalHandler(), teaFilter)
	_, err = p.Run()
	return err
}
