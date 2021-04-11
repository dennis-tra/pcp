package mdns

import (
	"context"

	"github.com/dennis-tra/pcp/internal/log"

	"github.com/libp2p/go-libp2p-core/host"
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

	for {
		did := a.DiscoveryID(chanID)
		log.Debugln("mDNS - Advertising ", did)
		ctx, cancel := context.WithTimeout(a.ServiceContext(), Timeout)
		mdns, err := wrapdiscovery.NewMdnsService(ctx, a, a.interval, did)
		if err != nil {
			cancel()
			return err
		}

		select {
		case <-a.SigShutdown():
			log.Debugln("mDNS - Advertising", did, " done - shutdown signal")
			cancel()
			return mdns.Close()
		case <-ctx.Done():
			log.Debugln("mDNS - Advertising", did, "done -", ctx.Err())
			cancel()
			if ctx.Err() == context.DeadlineExceeded {
				_ = mdns.Close()
				continue
			} else if ctx.Err() == context.Canceled {
				_ = mdns.Close()
				return nil
			}
		}
	}
}

func (a *Advertiser) Shutdown() {
	a.Service.Shutdown()
}
