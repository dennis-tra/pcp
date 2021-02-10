package discovery

import (
	"context"
	"sync"

	"github.com/libp2p/go-libp2p-core/peer"
)

type PeerHandler interface {
	HandlePeer(info peer.AddrInfo)
}

type Discoverer interface {
	Discover(ctx context.Context, identifier string, handler PeerHandler) error
	Stop() error
}

type BaseDiscoverer struct {
	lk       sync.Mutex
	notifees []Notifee
}

type Notifee interface {
	HandlePeerFound(peer.AddrInfo)
}

func (b *BaseDiscoverer) RegisterNotifee(n Notifee) {
	b.lk.Lock()
	b.notifees = append(b.notifees, n)
	b.lk.Unlock()
}

func (b *BaseDiscoverer) UnregisterNotifee(n Notifee) {
	b.lk.Lock()
	defer b.lk.Unlock()
	for i, notif := range b.notifees {
		if notif == n {
			b.notifees = append(b.notifees[:i], b.notifees[i+1:]...)
			return
		}
	}
}
