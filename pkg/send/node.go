package send

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/mdns"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

type Advertiser[T any] interface {
	Advertise(chanID int)
	State() T
	Error() error
	Shutdown()
}

var (
	_ Advertiser[mdns.State] = (*mdns.Advertiser)(nil)
	_ Advertiser[dht.State]  = (*dht.Advertiser)(nil)
)

// Node encapsulates the logic of advertising and transmitting
// a particular file to a peer.
type Node struct {
	*pcpnode.Node

	// mDNS advertisement implementations
	mdnsAdvertiser Advertiser[mdns.State]

	// DHT advertisement implementation
	dhtAdvertiser Advertiser[dht.State]

	// map of peers that passed PAKE
	authPeers *sync.Map

	// path to the file or directory to transfer
	filepath string

	// ....
	relayFinderActiveLk sync.RWMutex
	relayFinderActive   bool

	// reference to the ticker that triggers the status logs.
	statusTicker *time.Ticker

	// "closed" when the status log go routine returned
	logLoopWg sync.WaitGroup
}

// InitNode returns a fully configured node ready to start
// advertising that we want to send a specific file.
func InitNode(c *cli.Context, filepath string, words []string) (*Node, error) {
	var err error
	node := &Node{
		authPeers: &sync.Map{},
		filepath:  filepath,
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
		go node.logLoop(c.Bool("verbose"))
	}

	// register handler to respond to PAKE handshakes
	node.RegisterKeyExchangeHandler(node)

	return node, nil
}

func (n *Node) Shutdown() {
	n.stopAdvertising()
	n.UnregisterKeyExchangeHandler()
	n.Node.Shutdown()
	if n.statusTicker != nil {
		n.statusTicker.Stop()
	}
	n.logLoopWg.Wait()
}

func (n *Node) StartAdvertisingMDNS() {
	n.mdnsAdvertiser.Advertise(n.ChanID)
}

func (n *Node) StartAdvertisingDHT() {
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

// autoRelayPeerSource is a function that queries the DHT for a random peer ID with CPL 0.
// The found peers are used as candidates for circuit relay v2 peers.
func (n *Node) autoRelayPeerSource(ctx context.Context, num int) <-chan peer.AddrInfo {
	out := make(chan peer.AddrInfo)

	go func() {
		defer close(out)

		peerID, err := n.DHT.RoutingTable().GenRandPeerID(0)
		if err != nil {
			log.Warningln("error generating random peer ID:", err.Error())
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

func (n *Node) HandleSuccessfulKeyExchange(peerID peer.ID) {
	// We're authenticated so can initiate a transfer
	if n.GetState() == pcpnode.Connected {
		log.Debugln("already connected and authenticated with another node")
		return
	}
	n.SetState(pcpnode.Connected)

	n.UnregisterKeyExchangeHandler()
	n.stopAdvertising()

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
