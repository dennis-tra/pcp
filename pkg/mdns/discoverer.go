package mdns

import (
	"context"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/dennis-tra/pcp/internal/log"
)

type Discoverer struct {
	*protocol
	peerFoundHandler func(info peer.AddrInfo)
}

func NewDiscoverer(h host.Host) *Discoverer {
	return &Discoverer{protocol: newProtocol(h)}
}

func (d *Discoverer) Discover(chanID int, handler func(info peer.AddrInfo)) error {
	if err := d.ServiceStarted(); err != nil {
		return err
	}
	defer d.ServiceStopped()

	// TODO: locking? probably not as it's guarded by the service abstraction.
	d.peerFoundHandler = handler
	defer func() { d.peerFoundHandler = nil }()

	ctx := d.ServiceContext()

	did := d.DiscoveryID(chanID)
	log.Debugln("mDNS - Discovering", did)
	mdns := wrapdiscovery.NewMdnsService(d, did, d)
	if err := mdns.Start(); err != nil {
		return err
	}

	select {
	case <-d.SigShutdown():
		log.Debugln("mDNS - Discovering", did, " done - shutdown signal")
		if err := mdns.Close(); err != nil {
			log.Warningln("Error closing mdns service", err)
		}
		return nil
	case <-ctx.Done():
		log.Debugln("mDNS - Discovering", did, "done -", ctx.Err())
		if err := mdns.Close(); err != nil {
			log.Warningln("Error closing mdns service", err)
		}
		if ctx.Err() == context.Canceled {
			return nil
		}
		return ctx.Err()
	}
}

func (d *Discoverer) HandlePeerFound(pi peer.AddrInfo) {
	if d.peerFoundHandler == nil {
		return
	}

	log.Debugln("mDNS - Found peer", pi.ID)

	if pi.ID == d.ID() {
		return
	}

	pi.Addrs = onlyPrivate(pi.Addrs)
	if !isRoutable(pi) {
		return
	}

	go d.peerFoundHandler(pi)
}

func (d *Discoverer) Shutdown() {
	d.Service.Shutdown()
}

func isRoutable(pi peer.AddrInfo) bool {
	return len(pi.Addrs) > 0
}

// Filter out addresses that are public - only allow private ones.
func onlyPrivate(addrs []ma.Multiaddr) []ma.Multiaddr {
	routable := []ma.Multiaddr{}
	for _, addr := range addrs {
		if manet.IsPrivateAddr(addr) {
			routable = append(routable, addr)
			log.Debugf("\tprivate - %s\n", addr.String())
		} else {
			log.Debugf("\tpublic - %s\n", addr.String())
		}
	}
	return routable
}
