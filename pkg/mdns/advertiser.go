package mdns

import (
	"context"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/dennis-tra/pcp/internal/log"
)

type Advertiser struct {
	*protocol
}

func NewAdvertiser(h host.Host) *Advertiser {
	return &Advertiser{newProtocol(h)}
}

// Advertise broadcasts that we're providing data for the given code.
//
// TODO: NewMdnsService also polls for peers. This is quite chatty, so we could extract the server-only logic.
func (a *Advertiser) Advertise(chanID int) error {
	if err := a.ServiceStarted(); err != nil {
		return err
	}
	defer a.ServiceStopped()

	ctx := a.ServiceContext()

	did := a.DiscoveryID(chanID)
	log.Debugln("mDNS - Advertising ", did)
	mdns := wrapdiscovery.NewMdnsService(a, did, a)
	if err := mdns.Start(); err != nil {
		return err
	}

	select {
	case <-a.SigShutdown():
		log.Debugln("mDNS - Advertising", did, " done - shutdown signal")
		if err := mdns.Close(); err != nil {
			log.Warningln("Error closing mdns service", err)
		}
		return nil
	case <-ctx.Done():
		log.Debugln("mDNS - Advertising", did, "done -", ctx.Err())
		if err := mdns.Close(); err != nil {
			log.Warningln("Error closing mdns service", err)
		}
		if ctx.Err() == context.Canceled {
			return nil
		}
		return ctx.Err()
	}
}

func (a *Advertiser) Shutdown() {
	a.Service.Shutdown()
}

func (a *Advertiser) HandlePeerFound(info peer.AddrInfo) {
	// no-op
}
