package send

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/config"
)

// Command .
var Command = &cli.Command{
	Name:      "send",
	Usage:     "sends a file to a peer in your local network that is waiting to receive files",
	Aliases:   []string{"s"},
	Action:    Action,
	Flags:     []cli.Flag{},
	ArgsUsage: "FILE",
	UsageText: `FILE	The file you want to transmit to your peer (required).`,
	Description: `The send subcommand will look for multicast DNS services
that have registered in your local network. You will be able to choose
the desired peer or refresh the list.`,
}

// Action contains the logic for the send subcommand of the pcp program. It is
// mainly responsible for the TUI state handling and input parsing.
func Action(c *cli.Context) error {

	ctx, err := config.FillContext(c.Context)
	if err != nil {
		return err
	}

	// Try to open the file to check if we have access
	filepath := c.Args().First()
	if err = verifyFileAccess(filepath); err != nil {
		return err
	}

	local, err := InitNode(ctx)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to initialize node"))
	}
	defer local.Close()

	log.Infoln("Searching peers that are waiting to receive files...")
	err = local.StartMdnsService(ctx)
	if err != nil {
		return err
	}

	time.Sleep(local.MdnsInterval)

	peers := local.PeersList()
	log.Infof("\nFound the following peer(s):\n")
	local.PrintPeers(peers)

	for {
		if len(peers) == 0 {
			log.Info("No peer found in your local network [r,q,?]: ")
		} else {
			log.Info("Select the peer you want to send the file to [#,r,q,?]: ")
		}

		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return scanner.Err()
		}

		// sanitize user input
		input := strings.TrimSpace(scanner.Text())

		// Empty input, user just pressed enter => do nothing and prompt again
		if input == "" {
			continue
		}

		// Refresh the set of peers and prompt again
		if input == "r" {
			peers = local.PeersList()
			if len(peers) > 0 {
				log.Infof("\nFound the following peer(s):\n")
				local.PrintPeers(peers)
			}
			continue
		}

		// Quit the process
		if input == "q" {
			return nil
		}

		// Print the help text and prompt again
		if input == "?" {
			help()
			continue
		}

		if len(peers) == 0 {
			log.Infoln("Invalid input")
			continue
		}

		// Try to parse the input and
		num, err := strconv.Atoi(input)
		if err != nil {
			log.Infoln("Invalid input")
			continue
		} else if num >= len(peers) {
			log.Infoln("Peer index out of range")
			continue
		}

		// The user entered a valid peer index
		accepted, err := local.Transfer(ctx, peers[num], filepath)
		if err != nil {
			log.Infoln(err)
			continue
		} else if !accepted {
			continue
		}

		return nil
	}
}

// help prints the usage description for the user input in the "select peer" prompt.
func help() {
	log.Infoln("#: the number of the peer you want to connect to")
	log.Infoln("r: refresh peer list")
	log.Infoln("q: quit pcp")
	log.Infoln("?: this help message")
}
