package node

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dennis-tra/pcp/internal/app"
	"github.com/dennis-tra/pcp/pkg/commons"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery"
)

// The time a discovered peer will stay in the `Peers` map.
// If it doesn't get re-discovered within this time it gets
// removed from the list.
var gcDuration = 5 * time.Second

// Variable assignments for mocking purposes.
var (
	appDiscovery app.Discoverer = app.Discovery{}
	appTime      app.Timer      = app.Time{}
)

// MdnsProtocol encapsulates the logic for discovering peers
// in via multicast DNS in the local network.
type MdnsProtocol struct {
	node         *Node
	mdnsServ     discovery.Service
	MdnsInterval time.Duration
	Peers        *sync.Map
}

// NewMdnsProtocol creates a new MdnsProtocol struct with
// sane defaults.
func NewMdnsProtocol(node *Node) *MdnsProtocol {
	m := &MdnsProtocol{
		node:         node,
		MdnsInterval: time.Second,
		Peers:        &sync.Map{},
	}
	return m
}

// StartMdnsService starts broadcasting into the network that we are
// interested for the service tag given by commons.ServiceTag. The
// MdnsProtocol struct will get notified for newly discovered peers
// via HandlePeerFound. The local network is asked for peers with a
// frequency of one second. This function does nothing if the service
// is already running for this protocol.
func (m *MdnsProtocol) StartMdnsService(ctx context.Context) error {
	if m.mdnsServ != nil {
		return nil
	}

	mdns, err := appDiscovery.NewMdnsService(ctx, m.node.Host, time.Second, commons.ServiceTag)
	if err != nil {
		return err
	}
	m.mdnsServ = mdns

	mdns.RegisterNotifee(m)

	return nil
}

// StopMdnsService removes the MdnsProtocol from being notified for
// newly discoverd peers and shuts down the mDNS service. It also
// clears the list of peers.
func (m *MdnsProtocol) StopMdnsService() error {
	if m.mdnsServ == nil {
		return nil
	}

	m.mdnsServ.UnregisterNotifee(m)

	err := m.mdnsServ.Close()
	if err != nil {
		return err
	}

	m.Peers.Range(func(key, value interface{}) bool {
		value.(PeerInfo).timer.Stop()
		return true
	})

	m.mdnsServ = nil
	m.Peers = &sync.Map{}

	return nil
}

type PeerInfo struct {
	pi    peer.AddrInfo
	timer *time.Timer
}

// HandlePeerFound stores every newly found peer in a map.
// Every map entry gets a timer assigned that removes the
// entry after a garbage collection timeout if the peer
// is not seen again in the meantime. If we see the peer
// again we reset the time to start again from that point
// in time.
func (m *MdnsProtocol) HandlePeerFound(pi peer.AddrInfo) {
	savedPeer, ok := m.Peers.Load(pi.ID)
	if ok {
		savedPeer.(PeerInfo).timer.Reset(gcDuration)
	} else {
		t := appTime.AfterFunc(gcDuration, func() {
			m.Peers.Delete(pi.ID)
		})
		m.Peers.Store(pi.ID, PeerInfo{pi, t})
	}
}

// PeersList returns a sorted list of address information
// structs. Sorting order is based on the peer ID.
func (m *MdnsProtocol) PeersList() []peer.AddrInfo {
	peers := []peer.AddrInfo{}
	m.Peers.Range(func(key, value interface{}) bool {
		peers = append(peers, value.(PeerInfo).pi)
		return true
	})

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].ID < peers[j].ID
	})

	return peers
}

// PrintPeers dumps the given list of peers to the screen
// to be selected by the user via its index.
func (m *MdnsProtocol) PrintPeers(peers []peer.AddrInfo) {
	for i, p := range peers {
		fmt.Printf("[%d] %s\n", i, p.ID)
	}
	fmt.Println()
}
