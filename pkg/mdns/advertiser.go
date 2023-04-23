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
func (a *Advertiser) Advertise(chanID int) {
	if err := a.ServiceStarted(); err != nil {
		a.setError(err)
		return
	}
	defer a.ServiceStopped()

	a.setStage(StageRoaming)
	for {
		did := a.did.DiscoveryID(chanID)

		log.Debugln("mDNS - Advertising ", did)
		mdnsSvc := wrapdiscovery.NewMdnsService(a, did, a)
		if err := mdnsSvc.Start(); err != nil {
			a.setError(fmt.Errorf("start mdns service: %w", err))
			return
		}

		select {
		case <-a.SigShutdown():
			log.Debugln("mDNS - Advertising", did, " done - shutdown signal")
			if err := mdnsSvc.Close(); err != nil {
				log.Warningf("Error closing MDNS service: %s", err)
			}
			a.setStage(StageStopped)
			return
		case <-time.After(Timeout):
		}

		log.Debugln("mDNS - Advertising", did, "done")
		if err := mdnsSvc.Close(); err != nil {
			log.Warningf("Error closing MDNS service: %s", err)
		}
	}
}

func (a *Advertiser) HandlePeerFound(info peer.AddrInfo) {
	// no-op for advertisements - could consider to proactively contact other peers
}

func (a *Advertiser) Shutdown() {
	a.Service.Shutdown()
}
