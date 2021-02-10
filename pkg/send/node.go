package send

import (
	"context"
	"github.com/dennis-tra/pcp/internal/log"

	"github.com/dennis-tra/pcp/pkg/dht"
	pcpdiscovery "github.com/dennis-tra/pcp/pkg/discovery"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/network"
)

// Node encapsulates the logic of advertising and transmitting
// a particular file to a peer.
type Node struct {
	*pcpnode.Node
	advertisers []pcpdiscovery.Advertiser
}

// InitNode returns a fully configured node ready to start
// advertising that we want to send a specific file.
func InitNode(ctx context.Context) (*Node, error) {

	var err error
	h, err := pcpnode.Init(ctx, libp2p.EnableAutoRelay())
	if err != nil {
		return nil, err
	}

	advertisers := []pcpdiscovery.Advertiser{
		dht.NewAdvertiser(h),
		//mdns.NewAdvertiser(h),
	}

	node := &Node{Node: h, advertisers: advertisers}
	node.SetStreamHandler("/pcp/provide-request", node.onProvideRequest)

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

// Close stops all advertisers from broadcasting that we are providing
// the file we want to send and closes the active node.
func (n *Node) Close() {
	for _, advertiser := range n.advertisers {
		if err := advertiser.Stop(); err != nil {
			log.Warningln("Error stopping advertiser:", err)
		}
	}

	if err := n.Node.Close(); err != nil {
		log.Warningln("Error closing node", err)
	}
}

func (n *Node) onProvideRequest(s network.Stream) {

}

//func (n *Node) Transfer(ctx context.Context, pi peer.AddrInfo, filepath string) (bool, error) {
//
//	if err := n.Connect(ctx, pi); err != nil {
//		return false, err
//	}
//
//	c, err := calcContentID(filepath)
//	if err != nil {
//		return false, err
//	}
//
//	f, err := os.Open(filepath)
//	if err != nil {
//		return false, err
//	}
//	defer f.Close()
//
//	fstat, err := f.Stat()
//	if err != nil {
//		return false, err
//	}
//
//	log.Infof("Asking for confirmation... ")
//
//	accepted, err := n.SendPushRequest(ctx, pi.ID, path.Base(f.Name()), fstat.Size(), c)
//	if err != nil {
//		return false, err
//	}
//
//	if !accepted {
//		log.Infoln("Rejected!")
//		return accepted, nil
//	}
//	log.Infoln("Accepted!")
//
//	pr := progress.NewReader(f)
//
//	var wg sync.WaitGroup
//	wg.Add(1)
//
//	ctx, cancel := context.WithCancel(context.Background())
//	go pcpnode.IndicateProgress(ctx, pr, path.Base(f.Name()), fstat.Size(), &wg)
//	defer func() { cancel(); wg.Wait() }()
//
//	if _, err = n.Node.Transfer(ctx, pi.ID, pr); err != nil {
//		return accepted, errors.Wrap(err, "could not transfer file to peer")
//	}
//
//	log.Infoln("Successfully sent file!")
//	return accepted, nil
//}
