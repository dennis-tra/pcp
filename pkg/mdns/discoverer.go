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

func (d *Discoverer) Discover(chanID int) {
	if err := d.ServiceStarted(); err != nil {
		d.setError(err)
		return
	}
	defer d.ServiceStopped()

	d.setStage(StageRoaming)
	for {
		did := d.did.DiscoveryID(chanID)
		log.Debugln("mDNS - Discovering", did)

		mdnsSvc := wrapdiscovery.NewMdnsService(d, did, d)
		if err := mdnsSvc.Start(); err != nil {
			d.setError(fmt.Errorf("start mdns service: %w", err))
			return
		}

		select {
		case <-d.SigShutdown():
			log.Debugln("mDNS - Discovering", did, " done - shutdown signal")

			if err := mdnsSvc.Close(); err != nil {
				log.Warningf("Error closing MDNS service: %s", err)
			}
			d.setStage(StageStopped)
			return
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
	if len(pi.Addrs) == 0 {
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
