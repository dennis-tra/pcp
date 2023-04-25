package send

import (
	"context"
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

	// mDNS advertisement implementations
	mdnsAdvertiser *mdns.Advertiser

	// DHT advertisement implementation
	dhtAdvertiser *dht.Advertiser

	// path to the file or directory to transfer
	filepath string

	// ....
	relayFinderActiveLk sync.RWMutex
	relayFinderActive   bool

	// a logging service which updates the terminal with the current state
	statusLogger *statusLogger
}

// InitNode returns a fully configured node ready to start
// advertising that we want to send a specific file.
func InitNode(c *cli.Context, filepath string, words []string) (*Node, error) {
	var err error
	node := &Node{
		filepath: filepath,
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

	node.statusLogger = newStatusLogger(node)
	node.mdnsAdvertiser = mdns.NewAdvertiser(node)
	node.dhtAdvertiser = dht.NewAdvertiser(node, node.DHT)

	// start logging the current status to the console
	if !c.Bool("debug") {
		go node.statusLogger.startLogging()
	}

	// stop the process if both advertisers error out
	go node.watchAdvertiseErrors()

	// register handler to respond to PAKE handshakes
	node.RegisterKeyExchangeHandler(node)

	return node, nil
}

func (n *Node) Shutdown() {
	go func() {
		<-n.SigShutdown()

		n.stopAdvertising()
		n.UnregisterKeyExchangeHandler()
		n.statusLogger.Shutdown()

		if n.mdnsAdvertiser.State().Stage == mdns.StageError && n.dhtAdvertiser.State().Stage == dht.StageError {
			log.Infoln("An error occurred. Run peercp again with the --verbose flag to get more information")
		}

		// TODO: properly closing the host can take up to 1 minute
		//if err := n.Host.Close(); err != nil {
		//	log.Warningln("error stopping libp2p node:", err)
		//}

		n.ServiceStopped()
	}()

	// Blocks until ServiceStopped is called.
	// This call signals e.g., the statusLogger that the node is shutting down,
	// and we're cancelling the process.
	// That's why we put the other shutdown stuff in the go routine at the top.
	// If we called n.statusLogger.Shutdown() without shutting down this service
	// it wouldn't know that we cancelled the process and would just stop as normal.
	// What we want is, that it shows "cancelled" in the log output. That's why
	// we need the signal to be present when n.statusLogger.Shutdown() is called.
	n.Service.Shutdown()
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

		// if both advertisers errored out or one errored and the other is idle, stop the process
		if (mdnsState.Stage == mdns.StageError && (dhtState.Stage == dht.StageIdle || dhtState.Stage == dht.StageError)) ||
			(dhtState.Stage == dht.StageError && (mdnsState.Stage == mdns.StageIdle || mdnsState.Stage == mdns.StageError)) {

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

// HandleSuccessfulKeyExchange gets called when we have a peer that passed the
// password authenticated key exchange.
func (n *Node) HandleSuccessfulKeyExchange(peerID peer.ID) {
	// If there's a second peer that passed the password authenticated key exchange
	// we drop that peer because we already have one. Unlikely to happen.
	// SetState returns the previous state before the given "Connected" state was set.
	if n.SetState(pcpnode.Connected) == pcpnode.Connected {
		log.Debugln("already connected and authenticated with another peer")
		return
	}

	// Only if --debug is activated
	n.DebugLogAuthenticatedPeer(peerID)

	// we are connected to the correct peer:
	// 1. stop accepting key exchange requests
	// 2. stop advertising to the network that we're searching
	// 3. wait until we have a direct connection
	// 4. stop printing the search status
	n.UnregisterKeyExchangeHandler()
	n.stopAdvertising()

	err := n.WaitForDirectConn(peerID)
	if err != nil {
		n.statusLogger.Shutdown()
		n.Shutdown()
		log.Infoln("Hole punching failed:", err)
		return
	}
	n.statusLogger.Shutdown()

	// Start transferring file
	err = n.Transfer(peerID)
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
