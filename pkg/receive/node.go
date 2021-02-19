package receive

import (
	"bufio"
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	"github.com/dennis-tra/pcp/internal/format"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/mdns"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

type nodeState uint8

const (
	idle = iota
	discovering
	connected
)

type Node struct {
	*pcpnode.Node

	discoverers     []Discoverer
	discoveredPeers sync.Map

	stateLk sync.RWMutex
	state   nodeState
}

type Discoverer interface {
	Discover(handler pcpnode.PeerHandler) error
	Shutdown()
}

func InitNode(ctx context.Context, words []string) (*Node, error) {
	h, err := pcpnode.New(ctx, words)
	if err != nil {
		return nil, err
	}

	n := &Node{
		Node:            h,
		state:           idle,
		stateLk:         sync.RWMutex{},
		discoveredPeers: sync.Map{},
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

func (n *Node) StartDiscovering() {
	n.setState(discovering)

	// TODO: Implement rolling 5 minute window
	dhtKey1 := n.AdvertiseIdentifier(time.Now())
	dhtKey2 := n.AdvertiseIdentifier(time.Now().Add(-5 * time.Minute))

	n.discoverers = []Discoverer{
		dht.NewDiscoverer(n.Node, dhtKey1),
		dht.NewDiscoverer(n.Node, dhtKey2),
		mdns.NewDiscoverer(n.Node, dhtKey1),
		mdns.NewDiscoverer(n.Node, dhtKey2),
	}

	for _, discoverer := range n.discoverers {
		go func(d Discoverer) {
			if err := d.Discover(n); err != nil {
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

func (n *Node) setState(state nodeState) nodeState {
	n.stateLk.Lock()
	defer n.stateLk.Unlock()
	n.state = state
	return n.state
}

func (n *Node) getState() nodeState {
	n.stateLk.RLock()
	defer n.stateLk.RUnlock()
	return n.state
}

// HandlePeer is called async from the discoverers. It's okay to have long running tasks here.
func (n *Node) HandlePeer(pi peer.AddrInfo) {
	if n.getState() != discovering {
		log.Debugln("Received a peer from the discoverer although we're not discovering")
		return
	}

	// Check if we have already seen the peer and exit early to not connect again.
	// TODO: Check if the multi addresses have changed
	_, loaded := n.discoveredPeers.LoadOrStore(pi.ID, pi)
	if loaded {
		return
	}

	if err := n.Connect(n.ServiceContext(), pi); err != nil {
		// stale entry in DHT?
		// log.Debugln(err)
		return
	}

	// Negotiate PAKE
	if _, err := n.StartKeyExchange(n.ServiceContext(), pi.ID); err != nil {
		log.Errorln(err)
		return
	}

	// We're authenticated so can initiate a transfer
	if n.getState() == connected {
		log.Debugln("already connected and authenticated with another node")
		return
	}
	n.setState(connected)

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
