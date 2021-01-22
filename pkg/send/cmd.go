package send

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/urfave/cli/v2"
	"github.com/whyrusleeping/mdns"
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

	filepath := c.Args().First()
	if filepath == "" {
		return fmt.Errorf("please specify the file you want to send")
	}

	// Try to open the file to check if we have access
	f, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Println("Searching peers that are waiting to receive files...")
	peers, err := queryPeers()
	if err != nil {
		return err
	}

	for {
		if len(peers) == 0 {
			fmt.Print("No peer found in your local network [r,q,?]: ")
		} else {
			fmt.Printf("\nFound the following peer(s):\n")

			err = printPeers(peers)
			if err != nil {
				return err
			}

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
			peers, err = queryPeers()
			if err != nil {
				return err
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

		addrInfo, err := parseServiceEntry(peers[num])
		if err != nil {
			return err
		}

		// The user entered a valid peer index
		accepted, err := send(addrInfo, f)
		if err != nil {
			return err
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

// printPeers prints the service entries found through mDNS.
// It prefixes each entry with the index in the array.
func printPeers(peers []*mdns.ServiceEntry) error {
	for i, p := range peers {
		mpeer, err := peer.Decode(p.Info)
		if err != nil {
			return err
		}

		fmt.Printf("[%d] %s\n", i, mpeer.Pretty())
	}
	fmt.Println()
	return nil
}
