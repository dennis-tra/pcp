package dht

import (
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

type Discoverer struct {
	*protocol
	code string
}

func NewDiscoverer(node *pcpnode.Node, code string) *Discoverer {
	return &Discoverer{protocol: newProtocol(node), code: code}
}

func (d *Discoverer) Discover(handler pcpnode.PeerHandler) error {
	if err := d.ServiceStarted(); err != nil {
		return err
	}
	defer d.ServiceStopped()

	if err := d.Bootstrap(); err != nil {
		return err
	}

	contentID, err := strToCid(d.code)
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

		select {
		case <-d.SigShutdown():
			return nil
		default:
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
