package receive

import (
	"bufio"
	"context"
	"os"
	"strings"
	"sync"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	"github.com/dennis-tra/pcp/internal/format"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/dht"
	pcpdiscovery "github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/mdns"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/dennis-tra/pcp/pkg/words"
)

type nodeState uint8

const (
	idle = iota
	discovering
	connected
)

type Node struct {
	*pcpnode.Node
	*pcpnode.PakeClientProtocol

	discoverers []pcpdiscovery.Discoverer

	discoveredPeers sync.Map

	code    []string // transfer integers from the other peer
	stateLk sync.RWMutex
	state   nodeState
}

func InitNode(ctx context.Context, code []string) (*Node, error) {

	node, err := pcpnode.Init(ctx)
	if err != nil {
		return nil, err
	}

	pw, err := words.ToBytes(code)
	if err != nil {
		return nil, err
	}

	n := &Node{
		Node:            node,
		code:            code,
		discoveredPeers: sync.Map{},
		discoverers: []pcpdiscovery.Discoverer{
			dht.NewDiscoverer(node),
			mdns.NewDiscoverer(node),
		},
		PakeClientProtocol: pcpnode.NewPakeClientProtocol(node, pw),
	}

	n.RegisterPushRequestHandler(n)

	return n, nil
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

func (n *Node) Discover(ctx context.Context, code string) {
	n.setState(discovering)
	for _, discoverer := range n.discoverers {
		go discoverer.Discover(ctx, code, n)
	}
}

func (n *Node) StopDiscovering() {
	var wg sync.WaitGroup
	for _, discoverer := range n.discoverers {
		wg.Add(1)
		go func(d pcpdiscovery.Discoverer) {
			if err := d.Stop(); err != nil {
				log.Warningln(err)
			}
			wg.Done()
		}(discoverer)
	}
	wg.Wait()
}

func (n *Node) Shutdown() {
	n.StopDiscovering()

	n.UnregisterPushRequestHandler()
	n.UnregisterTransferHandler()

	n.Node.Shutdown()
}

// HandlePeer is called async from the discoverers. It's okay to have long running tasks here.
func (n *Node) HandlePeer(info peer.AddrInfo) {
	if n.getState() != discovering {
		log.Debugln("Received a peer from the discoverer although we're not discovering")
		return
	}

	// Check if we have already seen the peer and exit early to not connect again.
	// TODO: Check if the multi addresses have changed
	_, loaded := n.discoveredPeers.LoadOrStore(info.ID, info)
	if loaded {
		return
	}

	pubKey, err := n.Peerstore().PubKey(info.ID).Bytes()
	if err != nil {
		log.Errorln(err)
		return
	}

	// Check if the public key matches given words
	code, err := words.FromBytes(pubKey)
	if err != nil {
		log.Errorln(err)
		return
	}

	for i := range code {
		if n.code[i] != code[i] {
			log.Debugln("Pub Key of found peer does not match given word list")
			return
		}
	}

	err = n.Connect(n.Ctx(), info)
	if err != nil {
		// stale entry in DHT?
		// log.Debugln(err)
		return
	}

	// Negotiate PAKE
	_, err = n.StartKeyExchange(n.Ctx(), info.ID)
	if err != nil {
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
			th, err := NewTransferHandler(pr.Filename, pr.Size, pr.Cid, done)
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
		case <-n.Done():
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
