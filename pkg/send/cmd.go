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
	Name:      "send",
	Usage:     "sends a file to a peer in your local network that is waiting to receive files",
	ArgsUsage: "FILE",
	Aliases:   []string{"s"},
	Action:    Action,
}

// Action contains the logic for the send subcommand of the pcp program. It is
// mainly responsible for the TUI state handling and input parsing.
func Action(c *cli.Context) error {

	filepath := c.Args().First()
	if filepath == "" {
		return fmt.Errorf("please specify the file you want to send")
	}

	// Try to open the file and check if we have access
	f, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Println("Querying peers that are waiting to receive files...")
	peers, err := QueryPeers()
	if err != nil {
		return err
	}

	if len(peers) == 0 {
		return fmt.Errorf("no peer found in your local network")
	}

	fmt.Printf("\nFound the following peer(s):\n")

	err = printPeers(peers)
	if err != nil {
		return err
	}

	for {
		fmt.Print("Select the peer you want to send the file to [#,r,q,?]: ")

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
			peers, err = refresh()
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
		return send(addrInfo, f)
	}
}

// refresh asks the local network if there are peers waiting to receive files
// and then prints these peers if some are found.
func refresh() ([]*mdns.ServiceEntry, error) {
	peers, err := QueryPeers()
	if err != nil {
		return nil, err
	}

	err = printPeers(peers)
	if err != nil {
		return nil, err
	}

	return peers, nil
}

// help prints the usage description for the user input in the "select peer" prompt.
func help() {
	fmt.Println("Usage description here.")
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
