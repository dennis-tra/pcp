package receive

import (
	"bufio"
	"os"
	"strings"
	"sync"

	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/format"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/mdns"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
)

type PeerState uint8

const (
	NotConnected PeerState = iota
	Connecting
	Connected
	FailedConnecting
	FailedAuthentication
)

type Node struct {
	*pcpnode.Node

	autoAccept  bool
	discoverers []Discoverer
	peerStates  *sync.Map // TODO: Use PeerStore?
}

type Discoverer interface {
	Discover(chanID int, handler func(info peer.AddrInfo)) error
	Shutdown()
}

func InitNode(c *cli.Context, words []string) (*Node, error) {
	h, err := pcpnode.New(c, words)
	if err != nil {
		return nil, err
	}

	n := &Node{
		Node:        h,
		autoAccept:  c.Bool("auto-accept"),
		peerStates:  &sync.Map{},
		discoverers: []Discoverer{},
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
	peerState, _ := n.peerStates.LoadOrStore(pi.ID, NotConnected)
	switch peerState.(PeerState) {
	case NotConnected:
	case Connecting:
		log.Debugln("Skipping node as we're already trying to connect", pi.ID)
		return
	case FailedConnecting:
		// TODO: Check if multiaddrs have changed and only connect if that's the case
		log.Debugln("We tried to connect previously but couldn't establish a connection, try again", pi.ID)
	case FailedAuthentication:
		log.Debugln("We tried to connect previously but the node didn't pass authentication  -> skipping", pi.ID)
		return
	}

	log.Debugln("Connecting to peer:", pi.ID)
	n.peerStates.Store(pi.ID, Connecting)
	if err := n.Connect(n.ServiceContext(), pi); err != nil {
		log.Debugln("Error connecting to peer:", pi.ID, err)
		n.peerStates.Store(pi.ID, FailedConnecting)
		return
	}

	// Negotiate PAKE
	if _, err := n.StartKeyExchange(n.ServiceContext(), pi.ID); err != nil {
		log.Errorln("Peer didn't pass authentication:", err)
		n.peerStates.Store(pi.ID, FailedAuthentication)
		return
	}
	n.peerStates.Store(pi.ID, Connected)

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
	if n.autoAccept {
		return n.handleAccept(pr)
	}

	obj := "File"
	if pr.IsDir {
		obj = "Directory"
	}
	log.Infof("%s: %s (%s)\n", obj, pr.Name, format.Bytes(pr.Size))
	for {
		log.Infof("Do you want to receive this %s? [y,n,i,?] ", strings.ToLower(obj))
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
			return n.handleAccept(pr)
		}

		// Reject the file transfer
		if input == "n" {
			go n.Shutdown()
			return false, nil
		}

		log.Infoln("Invalid input")
	}
}

// handleAccept handles the case when the user accepted the transfer or provided
// the corresponding command line flag.
func (n *Node) handleAccept(pr *p2p.PushRequest) (bool, error) {
	done := n.TransferFinishHandler(pr.Size)
	th, err := NewTransferHandler(pr.Name, done)
	if err != nil {
		return true, err
	}
	n.RegisterTransferHandler(th)
	return true, nil
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
			log.Infoln("Successfully received file/directory!")
		} else {
			log.Infof("WARNING: Only received %d of %d bytes!\n", received, size)
		}

		n.Shutdown()
	}()
	return done
}
