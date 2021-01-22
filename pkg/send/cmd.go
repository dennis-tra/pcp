package send

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/urfave/cli/v2"
	"github.com/whyrusleeping/mdns"
)

// ServiceTag .
const ServiceTag = "pcp"

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

	err = PrintPeers(peers)
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

		// The user entered a valid peer index
		return send(peers[num], f)
	}
}

func send(targetPeer *mdns.ServiceEntry, contents io.Reader) error {

	fmt.Println("Selected peer: ", targetPeer.Info)

	fmt.Print("Establishing connection... ")
	ctx := context.Background()
	host, err := libp2p.New(ctx)

	targetPeerID, err := peer.Decode(targetPeer.Info)
	if err != nil {
		return fmt.Errorf("error parsing peer ID from mdns entry: %w", err)
	}

	var addr net.IP
	if targetPeer.AddrV4 != nil {
		addr = targetPeer.AddrV4
	} else if targetPeer.AddrV6 != nil {
		addr = targetPeer.AddrV6
	} else {
		return fmt.Errorf("error parsing multiaddr from mdns entry: no IP address found")
	}

	maddr, err := manet.FromNetAddr(&net.TCPAddr{IP: addr, Port: targetPeer.Port})
	if err != nil {
		return fmt.Errorf("error parsing multiaddr from mdns entry: %w", err)
	}

	pi := peer.AddrInfo{
		ID:    targetPeerID,
		Addrs: []ma.Multiaddr{maddr},
	}

	err = host.Connect(ctx, pi)
	if err != nil {
		return err
	}

	// open a stream, this stream will be handled by handleStream other end
	stream, err := host.NewStream(ctx, targetPeerID, ServiceTag)
	if err != nil {
		return fmt.Errorf("stream open faild: %w", err)
	}

	fmt.Println("Connected!")

	fmt.Println("Streaming file content to peer...")
	_, err = io.Copy(stream, contents)
	if err != nil {
		return err
	}

	err = stream.Close()
	if err != nil {
		return err
	}

	fmt.Println("Successfully sent file!")

	return nil
}

// refresh asks the local network if there are peers waiting to receive files
// and then prints these peers if some are found.
func refresh() ([]*mdns.ServiceEntry, error) {
	peers, err := QueryPeers()
	if err != nil {
		return nil, err
	}

	err = PrintPeers(peers)
	if err != nil {
		return nil, err
	}

	return peers, nil
}

// help prints the usage description for the user input in the "select peer" prompt.
func help() {
	fmt.Println("Usage description here.")
}

// QueryPeers will send DNS multicast messages in the local network to
// find all peers waiting to receive files.
func QueryPeers() ([]*mdns.ServiceEntry, error) {

	// TODO: Change this to an unbuffered channel. This is currently not
	// possible because the mdns library does an unblocking send into
	// their channel and therefore only one entry would be written
	// and subsequent sends will be dropped, because we don't get
	// the chance to drown the channel. Adjust the library:
	// https://github.com/dennis-tra/pcp/projects/1#card-53284716
	entriesCh := make(chan *mdns.ServiceEntry, 16)
	query := &mdns.QueryParam{
		Service:             ServiceTag,
		Domain:              "local",
		Timeout:             time.Second,
		Entries:             entriesCh,
		WantUnicastResponse: true,
	}

	err := mdns.Query(query)
	if err != nil {
		return nil, err
	}
	close(entriesCh)

	services := []*mdns.ServiceEntry{}
	for entry := range entriesCh {
		if _, err := peer.Decode(entry.Info); err != nil {
			continue
		}
		services = append(services, entry)
	}

	return services, nil
}

// PrintPeers prints the service entries found through mDNS.
// It prefixes each entry with the index in the array.
func PrintPeers(peers []*mdns.ServiceEntry) error {
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
