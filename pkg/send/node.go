package send

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/mdns"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

// Node encapsulates the logic of advertising and transmitting
// a particular file to a peer.
type Node struct {
	*pcpnode.Node

	// if verbose logging is activated
	verbose bool

	// mDNS advertisement implementations
	mdnsAdvertiser *mdns.Advertiser

	// DHT advertisement implementation
	dhtAdvertiser *dht.Advertiser

	// path to the file or directory to transfer
	filepath string

	// ....
	relayFinderActiveLk sync.RWMutex
	relayFinderActive   bool

	// if closed or sent a struct this channel will stop the print loop.
	stopPrintStatus chan struct{}

	// "closed" when the print-status go routine returned
	printStatusWg sync.WaitGroup
}

// InitNode returns a fully configured node ready to start
// advertising that we want to send a specific file.
func InitNode(c *cli.Context, filepath string, words []string) (*Node, error) {
	var err error
	node := &Node{
		verbose:         c.Bool("verbose"),
		filepath:        filepath,
		stopPrintStatus: make(chan struct{}),
	}

	opt := libp2p.EnableAutoRelayWithPeerSource(node.autoRelayPeerSource,
		autorelay.WithMetricsTracer(node),
		autorelay.WithBootDelay(0),
		autorelay.WithMinCandidates(1),
		autorelay.WithNumRelays(1),
	)

	node.Node, err = pcpnode.New(c, words, opt)
	if err != nil {
		return nil, err
	}

	node.mdnsAdvertiser = mdns.NewAdvertiser(node)
	node.dhtAdvertiser = dht.NewAdvertiser(node, node.DHT)

	// start logging the current status to the console
	if !c.Bool("debug") {
		go node.printStatus(node.stopPrintStatus)
	}

	// stop the process if both advertisers error out
	go node.watchAdvertiseErrors()

	// register handler to respond to PAKE handshakes
	node.RegisterKeyExchangeHandler(node)

	return node, nil
}

func (n *Node) Shutdown() {
	n.stopAdvertising()
	n.UnregisterKeyExchangeHandler()
	n.Node.Shutdown()
	close(n.stopPrintStatus)
	n.printStatusWg.Wait()
}

func (n *Node) StartAdvertisingMDNS() {
	n.SetState(pcpnode.Roaming)
	n.mdnsAdvertiser.Advertise(n.ChanID)
}

func (n *Node) StartAdvertisingDHT() {
	n.SetState(pcpnode.Roaming)
	n.dhtAdvertiser.Advertise(n.ChanID)
}

func (n *Node) stopAdvertising() {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		n.mdnsAdvertiser.Shutdown()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		n.dhtAdvertiser.Shutdown()
		wg.Done()
	}()

	wg.Wait()
}

func (n *Node) watchAdvertiseErrors() {
	for {
		select {
		case <-n.SigShutdown():
			return
		case <-n.mdnsAdvertiser.SigDone():
		case <-n.dhtAdvertiser.SigDone():
		}

		mdnsState := n.mdnsAdvertiser.State()
		dhtState := n.dhtAdvertiser.State()

		// if both advertisers errored out, stop the process
		if mdnsState.Stage == mdns.StageError && !errors.Is(mdnsState.Err, context.Canceled) &&
			dhtState.Stage == dht.StageError && !errors.Is(dhtState.Err, context.Canceled) {
			n.Shutdown()
			return
		}

		// if both advertisers reached a termination stage (e.g., both were stopped or one was stopped, the other
		// experienced an error), we have found and successfully connected to a peer. This means, all good - just
		// stop this go routine.
		if mdnsState.Stage.IsTermination() && dhtState.Stage.IsTermination() {
			return
		}
	}
}

// autoRelayPeerSource is a function that queries the DHT for a random peer ID with CPL 0.
// The found peers are used as candidates for circuit relay v2 peers.
func (n *Node) autoRelayPeerSource(ctx context.Context, num int) <-chan peer.AddrInfo {
	out := make(chan peer.AddrInfo)

	go func() {
		defer close(out)

		peerID, err := n.DHT.RoutingTable().GenRandPeerID(0)
		if err != nil {
			log.Debugln("error generating random peer ID:", err.Error())
			return
		}

		closestPeers, err := n.DHT.GetClosestPeers(ctx, peerID.String())
		if err != nil {
			return
		}

		maxLen := len(closestPeers)
		if maxLen > num {
			maxLen = num
		}

		for i := 0; i < maxLen; i++ {
			p := closestPeers[i]

			addrs := n.Peerstore().Addrs(p)
			if len(addrs) == 0 {
				continue
			}

			select {
			case out <- peer.AddrInfo{ID: p, Addrs: addrs}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// HandleSuccessfulKeyExchange gets called when we have a peer that passed the PAKE.
func (n *Node) HandleSuccessfulKeyExchange(peerID peer.ID) {
	// We're authenticated so can initiate a transfer
	if n.GetState() == pcpnode.Connected {
		log.Debugln("already connected and authenticated with another peer")
		return
	}
	n.SetState(pcpnode.Connected)

	n.UnregisterKeyExchangeHandler()
	n.stopAdvertising()

	// stop log loop
	n.stopPrintStatus <- struct{}{}
	n.printStatusWg.Wait()

	err := n.Transfer(peerID)
	if err != nil {
		log.Warningln("Error transferring file:", err)
	}

	n.Shutdown()
}

func (n *Node) Transfer(peerID peer.ID) error {
	filename := path.Base(n.filepath)
	size, err := totalSize(n.filepath)
	if err != nil {
		return err
	}

	log.Infof("Asking for confirmation... ")
	accepted, err := n.SendPushRequest(n.ServiceContext(), peerID, filename, size, false)
	if err != nil {
		return err
	}

	if !accepted {
		log.Infoln("Rejected!")
		return fmt.Errorf("rejected file transfer")
	}
	log.Infoln("Accepted!")

	if err = n.Node.Transfer(n.ServiceContext(), peerID, n.filepath); err != nil {
		return fmt.Errorf("could not transfer file to peer: %w", err)
	}

	log.Infoln("Successfully sent file/directory!")
	return nil
}

func totalSize(path string) (int64, error) {
	// TODO: Add file count
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size, err
}
