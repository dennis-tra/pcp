package mdns

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p/p2p/discovery"
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
		ctx, cancel := context.WithTimeout(a.ServiceContext(), time.Minute)
		mdns, err := discovery.NewMdnsService(ctx, a, a.interval, a.DiscoveryID(chanID))
		if err != nil {
			return err
		}

		select {
		case <-a.SigShutdown():
			cancel()
			return mdns.Close()
		case <-ctx.Done():
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
