package send

import (
	"context"
	"fmt"
	"github.com/dennis-tra/pcp/pkg/progress"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	"os"
	"path"
	"sync"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/dht"
	pcpdiscovery "github.com/dennis-tra/pcp/pkg/discovery"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	"github.com/dennis-tra/pcp/pkg/words"
)

// Node encapsulates the logic of advertising and transmitting
// a particular file to a peer.
type Node struct {
	*pcpnode.Node
	*pcpnode.PakeServerProtocol
	advertisers []pcpdiscovery.Advertiser

	authPeers sync.Map
	filepath  string

	TransferCode []string
	ChannelID    int16
}

// InitNode returns a fully configured node ready to start
// advertising that we want to send a specific file.
func InitNode(ctx context.Context, filepath string) (*Node, error) {

	var err error
	h, err := pcpnode.Init(ctx, libp2p.EnableAutoRelay())
	if err != nil {
		return nil, err
	}

	advertisers := []pcpdiscovery.Advertiser{
		dht.NewAdvertiser(h),
		//mdns.NewAdvertiser(h),
	}

	node := &Node{Node: h, advertisers: advertisers, authPeers: sync.Map{}, filepath: filepath}

	pubKey, err := node.Peerstore().PubKey(node.ID()).Bytes()
	if err != nil {
		return nil, err
	}

	tcode, err := words.FromBytes(pubKey)
	if err != nil {
		return nil, err
	}

	chanID, err := words.ToInt(tcode[0])
	if err != nil {
		return nil, err
	}

	node.ChannelID = chanID
	node.TransferCode = tcode

	pw, err := words.ToBytes(tcode)
	if err != nil {
		return nil, err
	}
	node.PakeServerProtocol = pcpnode.NewPakeServerProtocol(h, pw)
	node.PakeServerProtocol.RegisterKeyExchangeHandler(node)

	return node, nil
}

// Advertise asynchronously advertises the given code through the means of all
// registered advertisers. Currently these are multicast DNS and DHT.
func (n *Node) Advertise(ctx context.Context, code string) {
	for _, advertiser := range n.advertisers {
		go func(ad pcpdiscovery.Advertiser) {
			if err := ad.Advertise(ctx, code); err != nil {
				log.Warningln(err)
			}
		}(advertiser)
	}
}
func (n *Node) StopAdvertising() {
	var wg sync.WaitGroup
	for _, advertiser := range n.advertisers {
		wg.Add(1)
		go func(a pcpdiscovery.Advertiser) {
			if err := a.Stop(); err != nil {
				log.Warningln(err)
			}
			wg.Done()
		}(advertiser)
	}
	wg.Wait()
}

// Close stops all advertisers from broadcasting that we are providing
// the file we want to send and closes the active node.
func (n *Node) Close() {
	n.StopAdvertising()

	if err := n.Node.Close(); err != nil {
		log.Warningln("Error closing node", err)
	}
}

func (n *Node) HandleSuccessfulKeyExchange(peerID peer.ID) error {
	// TODO: Prevent calling this twice
	n.UnregisterKeyExchangeHandler()
	n.StopAdvertising()
	return n.Transfer(context.Background(), peerID)
}

func (n *Node) Transfer(ctx context.Context, peerID peer.ID) error {

	c, err := calcContentID(n.filepath)
	if err != nil {
		return err
	}

	f, err := os.Open(n.filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	fstat, err := f.Stat()
	if err != nil {
		return err
	}

	log.Infof("Asking for confirmation... ")

	accepted, err := n.SendPushRequest(ctx, peerID, path.Base(f.Name()), fstat.Size(), c)
	if err != nil {
		return err
	}

	if !accepted {
		log.Infoln("Rejected!")
		return fmt.Errorf("rejected file transfer")
	}
	log.Infoln("Accepted!")

	pr := progress.NewReader(f)

	var wg sync.WaitGroup
	wg.Add(1)

	ctx, cancel := context.WithCancel(context.Background())
	go pcpnode.IndicateProgress(ctx, pr, path.Base(f.Name()), fstat.Size(), &wg)
	defer func() { cancel(); wg.Wait() }()

	if _, err = n.Node.Transfer(ctx, peerID, pr); err != nil {
		return errors.Wrap(err, "could not transfer file to peer")
	}

	log.Infoln("Successfully sent file!")
	return nil
}
