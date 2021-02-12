package dht

import (
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/dennis-tra/pcp/pkg/discovery"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

type Discoverer struct {
	*protocol
}

func NewDiscoverer(node *pcpnode.Node) *Discoverer {
	return &Discoverer{protocol: newProtocol(node)}
}

func (d *Discoverer) Discover(code string, handler discovery.PeerHandler) error {
	if err := d.ServiceStarted(); err != nil {
		return err
	}
	defer d.ServiceStopped()

	if err := d.Bootstrap(); err != nil {
		return err
	}

	contentID, err := strToCid(code)
	if err != nil {
		return err
	}

	for {
		for pi := range d.DHT.FindProvidersAsync(
			d.ServiceContext(),
			contentID,
			100,
		) {
			pi.Addrs = onlyPublic(pi.Addrs)
			if isRoutable(pi) {
				go handler.HandlePeer(pi)
			}
		}
	}
}

func (d *Discoverer) Shutdown() {
	d.Service.Shutdown()
}

// Filter out addresses that are local - only allow public ones.
func onlyPublic(addrs []ma.Multiaddr) []ma.Multiaddr {
	routable := []ma.Multiaddr{}
	for _, addr := range addrs {
		if manet.IsPublicAddr(addr) {
			routable = append(routable, addr)
		}
	}
	return routable
}

func isRoutable(pi peer.AddrInfo) bool {
	return len(pi.Addrs) > 0
}
