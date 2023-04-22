package mdns

import (
	"fmt"
	"time"

	"github.com/dennis-tra/pcp/pkg/discovery"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/dennis-tra/pcp/internal/log"
)

type Discoverer struct {
	*protocol
	notifee discovery.Notifee
}

func NewDiscoverer(h host.Host, notifee discovery.Notifee) *Discoverer {
	return &Discoverer{
		protocol: newProtocol(h),
		notifee:  notifee,
	}
}

func (d *Discoverer) Discover(chanID int) error {
	if err := d.ServiceStarted(); err != nil {
		return err
	}
	defer d.ServiceStopped()

	for {
		did := d.did.DiscoveryID(chanID)
		log.Debugln("mDNS - Discovering", did)

		mdnsSvc := wrapdiscovery.NewMdnsService(d, did, d)
		if err := mdnsSvc.Start(); err != nil {
			return fmt.Errorf("start mdns service: %w", err)
		}

		select {
		case <-d.SigShutdown():
			log.Debugln("mDNS - Discovering", did, " done - shutdown signal")
			return mdnsSvc.Close()
		case <-time.After(Timeout):
		}

		log.Debugln("mDNS - Discovering", did, "done")
		if err := mdnsSvc.Close(); err != nil {
			log.Warningf("Error closing MDNS service: %s", err)
		}
	}
}

func (d *Discoverer) HandlePeerFound(pi peer.AddrInfo) {
	log.Debugln("mDNS - Found peer", pi.ID)

	if pi.ID == d.ID() {
		return
	}

	pi.Addrs = onlyPrivate(pi.Addrs)
	if !isRoutable(pi) {
		return
	}

	d.notifee.HandlePeerFound(pi)
}

func (d *Discoverer) SetOffset(offset time.Duration) *Discoverer {
	d.did.SetOffset(offset)
	return d
}

func (d *Discoverer) Shutdown() {
	d.Service.Shutdown()
}

// isRoutable returns true if we know at least one multi address
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
