package receive

import (
	"bufio"
	"context"
	"os"
	"strings"
	"sync"

	"github.com/dennis-tra/pcp/internal/format"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/mdns"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
)

type Node struct {
	*pcpnode.Node

	discoverers     []Discoverer
	discoveredPeers *sync.Map
}

type Discoverer interface {
	Discover(chanID int, handler func(info peer.AddrInfo)) error
	Shutdown()
}

func InitNode(ctx context.Context, words []string) (*Node, error) {
	h, err := pcpnode.New(ctx, words)
	if err != nil {
		return nil, err
	}

	n := &Node{
		Node:            h,
		discoveredPeers: &sync.Map{},
		discoverers:     []Discoverer{},
	}

	n.RegisterPushRequestHandler(n)

	return n, nil
}

func (n *Node) Shutdown() {
	n.StopDiscovering()
	n.UnregisterPushRequestHandler()
	n.UnregisterTransferHandler()
	n.Node.Shutdown()
}

func (n *Node) StartDiscovering(c *cli.Context) {
	n.SetState(pcpnode.Discovering)

	if c.Bool("mdns") == c.Bool("dht") {
		n.discoverers = []Discoverer{
			dht.NewDiscoverer(n, n.DHT),
			dht.NewDiscoverer(n, n.DHT).SetOffset(-dht.TruncateDuration),
			mdns.NewDiscoverer(n.Node),
			mdns.NewDiscoverer(n.Node).SetOffset(-dht.TruncateDuration),
		}
	} else if c.Bool("mdns") {
		n.discoverers = []Discoverer{
			mdns.NewDiscoverer(n.Node),
			mdns.NewDiscoverer(n.Node).SetOffset(-dht.TruncateDuration),
		}
	} else if c.Bool("dht") {
		n.discoverers = []Discoverer{
			dht.NewDiscoverer(n, n.DHT),
			dht.NewDiscoverer(n, n.DHT).SetOffset(-dht.TruncateDuration),
		}
	}

	for _, discoverer := range n.discoverers {
		go func(d Discoverer) {
			err := d.Discover(n.ChanID, n.HandlePeer)
			if err == nil {
				return
			}

			// If the user is connected to another peer
			// we don't care about discover errors.
			if n.GetState() == pcpnode.Connected {
				return
			}

			switch e := err.(type) {
			case dht.ErrConnThresholdNotReached:
				e.Log()
			default:
				log.Warningln(err)
			}
		}(discoverer)
	}
}

func (n *Node) StopDiscovering() {
	var wg sync.WaitGroup
	for _, discoverer := range n.discoverers {
		wg.Add(1)
		go func(d Discoverer) {
			d.Shutdown()
			wg.Done()
		}(discoverer)
	}
	wg.Wait()
}

// HandlePeer is called async from the discoverers. It's okay to have long running tasks here.
func (n *Node) HandlePeer(pi peer.AddrInfo) {
	if n.GetState() != pcpnode.Discovering {
		log.Debugln("Received a peer from the discoverer although we're not discovering")
		return
	}

	// Check if we have already seen the peer and exit early to not connect again.
	// TODO: Check if the multi addresses have changed
	_, loaded := n.discoveredPeers.LoadOrStore(pi.ID, pi)
	if loaded {
		log.Debugln("Skipping peer as we tried to connect previously:", pi.ID)
		return
	}

	log.Debugln("Connecting to peer:", pi.ID)
	if err := n.Connect(n.ServiceContext(), pi); err != nil {
		log.Debugln("Error connecting to peer:", pi.ID, err)
		return
	}

	// Negotiate PAKE
	if _, err := n.StartKeyExchange(n.ServiceContext(), pi.ID); err != nil {
		log.Errorln(err)
		return
	}

	// We're authenticated so can initiate a transfer
	if n.GetState() == pcpnode.Connected {
		log.Debugln("already connected and authenticated with another node")
		return
	}
	n.SetState(pcpnode.Connected)

	// Stop the discovering process as we have found the valid peer
	n.StopDiscovering()
}

func (n *Node) HandlePushRequest(pr *p2p.PushRequest) (bool, error) {
	log.Infof("File: %s (%s)\n", pr.Filename, format.Bytes(pr.Size))
	for {
		log.Infof("Do you want to receive this file? [y,n,i,?] ")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return true, errors.Wrap(scanner.Err(), "failed reading from stdin")
		}

		// sanitize user input
		input := strings.ToLower(strings.TrimSpace(scanner.Text()))

		// Empty input, user just pressed enter => do nothing and prompt again
		if input == "" {
			continue
		}

		// Print the help text and prompt again
		if input == "?" {
			help()
			continue
		}

		// Print information about the send request
		if input == "i" {
			printInformation(pr)
			continue
		}

		// Accept the file transfer
		if input == "y" {
			done := n.TransferFinishHandler(pr.Size)
			th, err := NewTransferHandler(pr.Filename, pr.Size, done)
			if err != nil {
				return true, err
			}
			n.RegisterTransferHandler(th)

			return true, nil
		}

		// Reject the file transfer
		if input == "n" {
			go n.Shutdown()
			return false, nil
		}

		log.Infoln("Invalid input")
	}
}

func (n *Node) TransferFinishHandler(size int64) chan int64 {
	done := make(chan int64)
	go func() {
		var received int64
		select {
		case <-n.SigShutdown():
			return
		case received = <-done:
		}

		if received == size {
			log.Infoln("Successfully received file!")
		} else {
			log.Infof("WARNING: Only received %d of %d bytes!\n", received, size)
		}

		n.Shutdown()
	}()
	return done
}
