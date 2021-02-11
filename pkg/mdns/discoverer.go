package mdns

import (
	"context"
	"net"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/pkg/errors"
	"github.com/whyrusleeping/mdns"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/discovery"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

type Discoverer struct {
	*protocol
	shutdown chan struct{}
	done     chan struct{}
}

func NewDiscoverer(node *pcpnode.Node) *Discoverer {
	return &Discoverer{
		protocol: newProtocol(node),
	}
}

func (d *Discoverer) Stop() error {
	if d.shutdown == nil {
		return nil
	}

	close(d.shutdown)
	<-d.done

	d.shutdown = nil
	d.done = nil

	return nil
}

func (d *Discoverer) Discover(ctx context.Context, identifier string, handler discovery.PeerHandler) error {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	d.shutdown = make(chan struct{})
	d.done = make(chan struct{})
	defer close(d.done)

	for {
		entriesCh := make(chan *mdns.ServiceEntry, 16)
		go d.drainEntriesChan(entriesCh, handler)

		qp := &mdns.QueryParam{
			Domain:  "local",
			Entries: entriesCh,
			Service: ServicePrefix + "/" + identifier, // keep in sync with advertiser
			Timeout: time.Second * 5,
		}

		err := mdns.Query(qp)
		if err != nil {
			log.Warningln("mdns lookup error", "error", err)
		}
		close(entriesCh)

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			continue
		case <-d.shutdown:
			return nil
		}
	}
}

func (d *Discoverer) drainEntriesChan(entries chan *mdns.ServiceEntry, handler discovery.PeerHandler) {
	for entry := range entries {

		pi, err := parseServiceEntry(entry)
		if err != nil {
			continue
		}

		if pi.ID == d.ID() {
			continue
		}

		// Filter out addresses that are public - only allow private ones.
		routable := []ma.Multiaddr{}
		for _, addr := range pi.Addrs {
			if manet.IsPrivateAddr(addr) {
				routable = append(routable, addr)
			}
		}

		if len(routable) == 0 {
			continue
		}

		pi.Addrs = routable
		go handler.HandlePeer(pi)
	}
}

func parseServiceEntry(entry *mdns.ServiceEntry) (peer.AddrInfo, error) {
	p, err := peer.Decode(entry.Info)
	if err != nil {
		return peer.AddrInfo{}, errors.Wrap(err, "error parsing peer ID from mdns entry")
	}

	var addr net.IP
	if entry.AddrV4 != nil {
		addr = entry.AddrV4
	} else if entry.AddrV6 != nil {
		addr = entry.AddrV6
	} else {
		return peer.AddrInfo{}, errors.Wrap(err, "error parsing multiaddr from mdns entry: no IP address found")
	}

	maddr, err := manet.FromNetAddr(&net.TCPAddr{IP: addr, Port: entry.Port})
	if err != nil {
		return peer.AddrInfo{}, errors.Wrap(err, "error parsing multiaddr from mdns entry")
	}

	return peer.AddrInfo{
		ID:    p,
		Addrs: []ma.Multiaddr{maddr},
	}, nil
}

//// The time a discovered peer will stay in the `peers` map.
//// If it doesn't get re-discovered within this time it gets
//// removed from the list.
//var gcDuration = 5 * time.Second

//func (m *Discoverer) StopMonitoring() error {
//
//	m.peers.Range(func(key, value interface{}) bool {
//		value.(PeerInfo).timer.Stop()
//		return true
//	})
//
//	m.peers = &sync.Map{}
//	close(m.shutdown)
//
//	return nil
//}

//type PeerInfo struct {
//	pi    peer.AddrInfo
//	timer *time.Timer
//}
//
//// HandlePeerFound stores every newly found peer in a map.
//// Every map entry gets a timer assigned that removes the
//// entry after a garbage collection timeout if the peer
//// is not seen again in the meantime. If we see the peer
//// again we reset the time to start again from that point
//// in time.
//func (m *Discoverer) HandlePeerFound(pi peer.AddrInfo) {
//	savedPeer, ok := m.peers.Load(pi.ID)
//	if ok {
//		savedPeer.(PeerInfo).timer.Reset(gcDuration)
//	} else {
//		t := appTime.AfterFunc(gcDuration, func() {
//			m.peers.Delete(pi.ID)
//		})
//		m.peers.Store(pi.ID, PeerInfo{pi, t})
//	}
//}
//
//// PeersList returns a sorted list of address information
//// structs. Sorting order is based on the peer ID.
//func (m *Discoverer) PeersList() []peer.AddrInfo {
//	peers := []peer.AddrInfo{}
//	m.peers.Range(func(key, value interface{}) bool {
//		peers = append(peers, value.(PeerInfo).pi)
//		return true
//	})
//
//	sort.Slice(peers, func(i, j int) bool {
//		return peers[i].ID < peers[j].ID
//	})
//
//	return peers
//}
//
//// PrintPeers dumps the given list of peers to the screen
//// to be selected by the user via its index.
//func (m *Discoverer) PrintPeers(peers []peer.AddrInfo) {
//	for i, p := range peers {
//		log.Infof("[%d] %s\n", i, p.ID)
//	}
//	log.Infoln()
//}
