package send

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-protocol"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
	"github.com/urfave/cli/v2"
	"github.com/whyrusleeping/mdns"
)

// ServiceTag .
const ServiceTag = "pcp"

// Command .
var Command = &cli.Command{
	Name:    "send",
	Aliases: []string{"s"},
	Action:  Action,
}

func validateInput(input string, peerCount int) (int, error) {
	if len(input) == 0 {
		return 0, fmt.Errorf("")
	}

	num, err := strconv.Atoi(input)
	if err != nil {
		return 0, fmt.Errorf("Invalid input")
	}

	if num >= peerCount {
		return 0, fmt.Errorf("Peer index out of range")
	}

	return num, nil
}

// Action contains the logic for the send subcommand of the pcp program.
func Action(c *cli.Context) error {

	fmt.Println("Querying peers that are waiting to receive files...")
	peers, err := QueryPeers()
	if err != nil {
		return err
	}

	fmt.Printf("\nFound the following peer(s):\n")

	err = PrintPeers(peers)
	if err != nil {
		return err
	}

	var targetPeer *mdns.ServiceEntry

INPUT_LOOP:
	for {
		fmt.Print("\nSelect the peer you want to send the file to [#,r,q,?]: ")

		scanner := bufio.NewScanner(os.Stdin)
		scanResult := scanner.Scan()

		if !scanResult {
			return scanner.Err()
		}

		switch scanner.Text() {
		case "r":
			peers, err = QueryPeers()
			if err != nil {
				return err
			}

			err = PrintPeers(peers)
			if err != nil {
				return err
			}

		case "q":
			return nil

		case "?":
			fmt.Println("Print help")

		default:

			num, err := validateInput(scanner.Text(), len(peers))
			if err != nil {
				fmt.Println(err.Error())
				continue
			}

			targetPeer = peers[num]

			break INPUT_LOOP
		}
	}

	fmt.Println("sending Data to peer", targetPeer.Info)

	// ci, err := cid.Decode(c.Args().First())
	// if err != nil {
	// 	return err
	// }

	// fmt.Println(ci.String())

	ctx := context.Background()
	host, err := libp2p.New(ctx)

	targetPeerID, err := peer.IDB58Decode(targetPeer.Info)
	if err != nil {
		return fmt.Errorf("Error parsing peer ID from mdns entry: %s", err)
	}

	var addr net.IP
	if targetPeer.AddrV4 != nil {
		addr = targetPeer.AddrV4
	} else if targetPeer.AddrV6 != nil {
		addr = targetPeer.AddrV6
	} else {
		return fmt.Errorf("Error parsing multiaddr from mdns entry: no IP address found")
	}

	maddr, err := manet.FromNetAddr(&net.TCPAddr{IP: addr, Port: targetPeer.Port})
	if err != nil {
		return fmt.Errorf("Error parsing multiaddr from mdns entry: %s", err)
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
	stream, err := host.NewStream(ctx, targetPeerID, protocol.ID(ServiceTag))
	if err != nil {
		return fmt.Errorf("stream open faild: %s", err)
	}

	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	go writeData(rw)
	go readData(rw)

	fmt.Println("Connected to:", targetPeerID)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)

	select {
	case <-stop:
		host.Close()
		os.Exit(0)
	}

	return nil
}

// QueryPeers will send a DNS multicast message to find
// all peers waiting to receive files.
func QueryPeers() ([]*mdns.ServiceEntry, error) {

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
		services = append(services, entry)
	}

	return services, nil
}

// PrintPeers prints the service entries found through mDNS.
// It prefixes each entry with the index in the array.
func PrintPeers(peers []*mdns.ServiceEntry) error {
	for i, p := range peers {
		mpeer, err := peer.IDB58Decode(p.Info)
		if err != nil {
			return err
		}

		fmt.Printf("[%d] %s\n", i, mpeer.Pretty())
	}
	return nil
}

func readData(rw *bufio.ReadWriter) {
	for {
		str, err := rw.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from buffer")
			panic(err)
		}

		if str == "" {
			return
		}
		if str != "\n" {
			// Green console colour: 	\x1b[32m
			// Reset console colour: 	\x1b[0m
			fmt.Printf("\x1b[32m%s\x1b[0m> ", str)
		}

	}
}

func writeData(rw *bufio.ReadWriter) {
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from stdin")
			panic(err)
		}

		_, err = rw.WriteString(fmt.Sprintf("%s\n", sendData))
		if err != nil {
			fmt.Println("Error writing to buffer")
			panic(err)
		}
		err = rw.Flush()
		if err != nil {
			fmt.Println("Error flushing buffer")
			panic(err)
		}
	}
}
