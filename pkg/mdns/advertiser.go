package mdns

import (
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/dennis-tra/pcp/internal/log"
)

type Advertiser struct {
	*protocol
}

func NewAdvertiser(h host.Host) *Advertiser {
	return &Advertiser{newProtocol(h)}
}

// Advertise broadcasts that we're providing data for the given channel ID in the local network.
func (a *Advertiser) Advertise(chanID int) error {
	if err := a.ServiceStarted(); err != nil {
		return err
	}
	defer a.ServiceStopped()

	for {
		did := a.did.DiscoveryID(chanID)
		log.Debugln("mDNS - Advertising ", did)
		mdns := wrapdiscovery.NewMdnsService(a, did, a)
		if err := mdns.Start(); err != nil {
			return fmt.Errorf("start mdns service: %w", err)
		}

		select {
		case <-a.SigShutdown():
			log.Debugln("mDNS - Advertising", did, " done - shutdown signal")
			return mdns.Close()
		case <-time.After(Timeout):
		}

		log.Debugln("mDNS - Advertising", did, "done")
		if err := mdns.Close(); err != nil {
			log.Warningf("Error closing MDNS service: %s", err)
		}
	}
}

func (a *Advertiser) HandlePeerFound(info peer.AddrInfo) {
}

func (a *Advertiser) Shutdown() {
	a.Service.Shutdown()
}
