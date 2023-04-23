package receive

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/dennis-tra/pcp/pkg/discovery"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/format"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/mdns"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
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

	// if verbose logging is activated
	verbose bool

	// mDNS discovery implementations
	mdnsDiscoverer       *mdns.Discoverer
	mdnsDiscovererOffset *mdns.Discoverer

	// DHT discovery implementations
	dhtDiscoverer       *dht.Discoverer
	dhtDiscovererOffset *dht.Discoverer

	autoAccept bool
	peerStates *sync.Map // TODO: Use PeerStore?

	// if closed or sent a struct this channel will stop the print loop.
	stopPrintStatus chan struct{}

	// "closed" when the print-status go routine returned
	printStatusWg sync.WaitGroup
}

func InitNode(c *cli.Context, words []string) (*Node, error) {
	h, err := pcpnode.New(c, words)
	if err != nil {
		return nil, err
	}

	node := &Node{
		Node:       h,
		verbose:    c.Bool("verbose"),
		autoAccept: c.Bool("auto-accept"),
		peerStates: &sync.Map{},
	}

	node.mdnsDiscoverer = mdns.NewDiscoverer(node, node)
	node.mdnsDiscovererOffset = mdns.NewDiscoverer(node, node).SetOffset(-discovery.TruncateDuration)
	node.dhtDiscoverer = dht.NewDiscoverer(node, node.DHT, node)
	node.dhtDiscovererOffset = dht.NewDiscoverer(node, node.DHT, node).SetOffset(-discovery.TruncateDuration)

	node.RegisterPushRequestHandler(node)
	// start logging the current status to the console

	if !c.Bool("debug") {
		go node.printStatus(node.stopPrintStatus)
	}

	// stop the process if all discoverers error out
	go node.watchDiscoverErrors()

	return node, nil
}

func (n *Node) Shutdown() {
	n.stopDiscovering()
	n.UnregisterPushRequestHandler()
	n.UnregisterTransferHandler()
	n.Node.Shutdown()
}

func (n *Node) StartDiscoveringMDNS() {
	n.SetState(pcpnode.Roaming)
	n.mdnsDiscoverer.Discover(n.ChanID)
	n.mdnsDiscovererOffset.Discover(n.ChanID)
}

func (n *Node) StartDiscoveringDHT() {
	n.SetState(pcpnode.Roaming)
	n.dhtDiscoverer.Discover(n.ChanID)
	n.dhtDiscovererOffset.Discover(n.ChanID)
}

func (n *Node) stopDiscovering() {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		n.mdnsDiscoverer.Shutdown()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		n.mdnsDiscovererOffset.Shutdown()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		n.dhtDiscoverer.Shutdown()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		n.dhtDiscovererOffset.Shutdown()
		wg.Done()
	}()

	wg.Wait()
}

func (n *Node) watchDiscoverErrors() {
	for {
		select {
		case <-n.SigShutdown():
			return
		case <-n.mdnsDiscoverer.SigDone():
		case <-n.mdnsDiscovererOffset.SigDone():
		case <-n.dhtDiscoverer.SigDone():
		case <-n.dhtDiscovererOffset.SigDone():
		}
		mdnsState := n.mdnsDiscoverer.State()
		mdnsOffsetState := n.mdnsDiscovererOffset.State()
		dhtState := n.dhtDiscoverer.State()
		dhtOffsetState := n.dhtDiscovererOffset.State()

		// if all discoverers errored out, stop the process
		if mdnsState.Stage == mdns.StageError && mdnsOffsetState.Stage == mdns.StageError &&
			dhtState.Stage == dht.StageError && dhtOffsetState.Stage == dht.StageError {
			n.Shutdown()
			return
		}

		// if all discoverers reached a termination stage (e.g., both were stopped or one was stopped, the other
		// experienced an error), we have found and successfully connected to a peer. This means, all good - just
		// stop this go routine.
		if mdnsState.Stage.IsTermination() && mdnsOffsetState.Stage.IsTermination() &&
			dhtState.Stage.IsTermination() && dhtOffsetState.Stage.IsTermination() {
			n.Shutdown()
			return
		}
	}
}

// HandlePeerFound is called async from the discoverers. It's okay to have long-running tasks here.
func (n *Node) HandlePeerFound(pi peer.AddrInfo) {
	if n.GetState() != pcpnode.Roaming {
		log.Debugln("Received a peer from the discoverer although we're not discovering")
		return
	}

	// Add discovered peer to the hole punch allow list to track the hole punch state of that
	// particular peer as soon as we try to connect to them.
	n.AddToHolePunchAllowList(pi.ID)

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
	n.stopDiscovering()
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
			return true, fmt.Errorf("failed reading from stdin: %w", scanner.Err())
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
