package send

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/dht"
	pcpdiscovery "github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/mdns"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	"github.com/dennis-tra/pcp/pkg/progress"
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

	h, err := pcpnode.New(ctx, libp2p.EnableAutoRelay())
	if err != nil {
		return nil, err
	}

	node := &Node{Node: h, advertisers: []pcpdiscovery.Advertiser{}, authPeers: sync.Map{}, filepath: filepath}

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

func (n *Node) Shutdown() {
	n.StopAdvertising()
	n.UnregisterKeyExchangeHandler()
	n.Node.Shutdown()
}

// Advertise asynchronously advertises the given code through the means of all
// registered advertisers. Currently these are multicast DNS and DHT.
func (n *Node) Advertise(code string) {

	n.advertisers = []pcpdiscovery.Advertiser{
		dht.NewAdvertiser(n.Node),
		mdns.NewAdvertiser(n.Node),
	}

	for _, advertiser := range n.advertisers {
		go func(a pcpdiscovery.Advertiser) {
			if err := a.Advertise(code); err != nil {
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
			a.Shutdown()
			wg.Done()
		}(advertiser)
	}
	wg.Wait()
}

func (n *Node) HandleSuccessfulKeyExchange(peerID peer.ID) {
	// TODO: Prevent calling this twice
	n.UnregisterKeyExchangeHandler()
	go n.StopAdvertising()

	err := n.Transfer(peerID)
	if err != nil {
		log.Warningln("Error transferring file", err)
	}

	n.Shutdown()
}

func (n *Node) Transfer(peerID peer.ID) error {

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

	accepted, err := n.SendPushRequest(n.ServiceContext(), peerID, path.Base(f.Name()), fstat.Size(), c)
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

	ctx, cancel := context.WithCancel(n.ServiceContext())
	go pcpnode.IndicateProgress(ctx, pr, path.Base(f.Name()), fstat.Size(), &wg)
	defer func() { cancel(); wg.Wait() }()

	if _, err = n.Node.Transfer(ctx, peerID, pr); err != nil {
		return errors.Wrap(err, "could not transfer file to peer")
	}

	log.Infoln("Successfully sent file!")
	return nil
}
