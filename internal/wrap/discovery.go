package wrap

import (
	"context"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

type Discoverer interface {
	NewMdnsService(ctx context.Context, peerhost host.Host, serviceTag string) (mdns.Service, error)
}

type Discovery struct{}

func (d Discovery) HandlePeerFound(peer.AddrInfo) {}
func (d Discovery) NewMdnsService(ctx context.Context, peerhost host.Host, serviceTag string) (mdns.Service, error) {
	mdns := mdns.NewMdnsService(peerhost, serviceTag, &d)
	if err := mdns.Start(); err != nil {
		mdns.Close()
		return nil, err
	}
	return mdns, nil
}
