package send

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
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

	advertisers []Advertiser

	authPeers *sync.Map
	filepath  string
}

type Advertiser interface {
	Advertise(chanID int) error
	Shutdown()
}

// InitNode returns a fully configured node ready to start
// advertising that we want to send a specific file.
func InitNode(c *cli.Context, filepath string, words []string) (*Node, error) {
	h, err := pcpnode.New(c, words, libp2p.EnableAutoRelay())
	if err != nil {
		return nil, err
	}

	node := &Node{
		Node:        h,
		advertisers: []Advertiser{},
		authPeers:   &sync.Map{},
		filepath:    filepath,
	}

	node.RegisterKeyExchangeHandler(node)

	return node, nil
}

func (n *Node) Shutdown() {
	n.StopAdvertising()
	n.UnregisterKeyExchangeHandler()
	n.Node.Shutdown()
}

// StartAdvertising asynchronously advertises the given code through the means of all
// registered advertisers. Currently these are multicast DNS and DHT.
func (n *Node) StartAdvertising(c *cli.Context) {
	n.SetState(pcpnode.Advertising)

	if c.Bool("mdns") == c.Bool("dht") {
		n.advertisers = []Advertiser{
			dht.NewAdvertiser(n, n.DHT),
			mdns.NewAdvertiser(n.Node),
		}
	} else if c.Bool("mdns") {
		n.advertisers = []Advertiser{
			mdns.NewAdvertiser(n.Node),
		}
	} else if c.Bool("dht") {
		n.advertisers = []Advertiser{
			dht.NewAdvertiser(n, n.DHT),
		}
	}

	for _, advertiser := range n.advertisers {
		go func(a Advertiser) {
			err := a.Advertise(n.ChanID)
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
		}(advertiser)
	}
}

func (n *Node) StopAdvertising() {
	var wg sync.WaitGroup
	for _, advertiser := range n.advertisers {
		wg.Add(1)
		go func(a Advertiser) {
			a.Shutdown()
			wg.Done()
		}(advertiser)
	}
	wg.Wait()
}

func (n *Node) HandleSuccessfulKeyExchange(peerID peer.ID) {
	// We're authenticated so can initiate a transfer
	if n.GetState() == pcpnode.Connected {
		log.Debugln("already connected and authenticated with another node")
		return
	}
	n.SetState(pcpnode.Connected)

	n.UnregisterKeyExchangeHandler()
	go n.StopAdvertising()

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
		return errors.Wrap(err, "could not transfer file to peer")
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
