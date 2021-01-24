package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dennis-tra/pcp/pkg/commons"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery"
)

const gcDuration = 5 * time.Second

// MdnsProtocol type
type MdnsProtocol struct {
	node         *Node
	mdnsServ     discovery.Service
	MdnsInterval time.Duration
	Peers        sync.Map
}

func NewMdnsProtocol(node *Node) *MdnsProtocol {
	m := &MdnsProtocol{
		node:         node,
		MdnsInterval: time.Second,
		Peers:        sync.Map{},
	}
	return m
}

func (m *MdnsProtocol) StartMdnsService(ctx context.Context) error {
	if m.mdnsServ != nil {
		return nil
	}

	mdns, err := discovery.NewMdnsService(ctx, m.node.Host, time.Second, commons.ServiceTag)
	if err != nil {
		return err
	}
	m.mdnsServ = mdns

	mdns.RegisterNotifee(m)

	return nil
}

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
	m.Peers = sync.Map{}

	return nil
}

type PeerInfo struct {
	pi    peer.AddrInfo
	timer *time.Timer
}

func (m *MdnsProtocol) HandlePeerFound(pi peer.AddrInfo) {
	savedPeer, ok := m.Peers.Load(pi.ID)
	if ok {
		savedPeer.(PeerInfo).timer.Reset(gcDuration)
	} else {
		t := time.AfterFunc(gcDuration, func() {
			m.Peers.Delete(pi.ID)
		})
		m.Peers.Store(pi.ID, PeerInfo{pi, t})
	}
}

func (m *MdnsProtocol) PeersList() []peer.AddrInfo {
	peers := []peer.AddrInfo{}
	m.Peers.Range(func(key, value interface{}) bool {
		peers = append(peers, value.(PeerInfo).pi)
		return true
	})

	// TODO: Sort

	return peers
}

func (m *MdnsProtocol) PrintPeers(peers []peer.AddrInfo) {
	for i, p := range peers {
		fmt.Printf("[%d] %s\n", i, p.ID)
	}
	fmt.Println()
}
