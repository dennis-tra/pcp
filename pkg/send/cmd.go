package send

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/urfave/cli/v2"
)

// Command .
var Command = &cli.Command{
	Name:    "send",
	Usage:   "sends a file to a peer in your local network that is waiting to receive files",
	Aliases: []string{"s"},
	Action:  Action,
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:    "timeout",
			EnvVars: []string{"PCP_MDNS_TIMEOUT"},
			Aliases: []string{"t"},
			Usage:   "The timeout to wait for multicast DNS replies in seconds.",
			Value:   1,
		},
	},
	ArgsUsage: "FILE",
	UsageText: `FILE	The file you want to transmit to your peer (required).`,
	Description: `The send subcommand will look for multicast DNS services
that have registered in your local network. You will be able to choose
the desired peer or refresh the list.`,
}

// Action contains the logic for the send subcommand of the pcp program. It is
// mainly responsible for the TUI state handling and input parsing.
func Action(c *cli.Context) error {

	conf, err := config.LoadConfig()
	if err != nil {
		return err
	}
	ctx := context.WithValue(c.Context, config.ContextKey, conf)

	// Try to open the file to check if we have access
	filepath := c.Args().First()
	err = verifyFileAccess(filepath)
	if err != nil {
		return err
	}

	n, err := InitNode(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Searching peers that are waiting to receive files...")
	err = n.StartMdnsService(ctx)
	if err != nil {
		return err
	}

	time.Sleep(n.MdnsInterval)

	peers := n.PeersList()
	n.PrintPeers(peers)

	for {
		if len(peers) == 0 {
			fmt.Print("No peer found in your local network [r,q,?]: ")
		} else {
			fmt.Print("Select the peer you want to send the file to [#,r,q,?]: ")
		}

		scanner := bufio.NewScanner(os.Stdin)
		scanResult := scanner.Scan()

		if !scanResult {
			return scanner.Err()
		}

		input := strings.TrimSpace(scanner.Text())

		// Empty input, user just pressed enter => do nothing and prompt again
		if input == "" {
			continue
		}

		// Refresh the set of peers and prompt again
		if input == "r" {
			peers = n.PeersList()
			if len(peers) > 0 {
				fmt.Printf("\nFound the following peer(s):\n")
				n.PrintPeers(peers)
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
			fmt.Println("Invalid input")
			continue
		}

		// Try to parse the input and
		num, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println("Invalid input")
			continue
		} else if num >= len(peers) {
			fmt.Println("Peer index out of range")
			continue
		}

		// The user entered a valid peer index
		accepted, err := n.send(ctx, peers[num], filepath)
		if err != nil {
			fmt.Println(err)
			continue
		} else if !accepted {
			continue
		}

		return nil
	}
}

// help prints the usage description for the user input in the "select peer" prompt.
func help() {
	fmt.Println("#: the number of the peer you want to connect to")
	fmt.Println("r: refresh peer list")
	fmt.Println("q: quit pcp")
	fmt.Println("?: this help message")
}

func verifyFileAccess(filepath string) error {

	if filepath == "" {
		return fmt.Errorf("please specify the file you want to send")
	}

	f, err := os.Open(filepath)
	if err != nil {
		return err
	}

	return f.Close()
}
