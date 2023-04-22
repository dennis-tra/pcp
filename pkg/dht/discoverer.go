package dht

import (
	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/libp2p/go-libp2p/core/host"
)

import (
	"context"
	"time"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Discoverer is responsible for reading the DHT for an
// entry with the channel ID given below.
type Discoverer struct {
	*protocol

	notifee discovery.Notifee
}

// NewDiscoverer creates a new Discoverer.
func NewDiscoverer(h host.Host, dht wrap.IpfsDHT, notifee discovery.Notifee) *Discoverer {
	return &Discoverer{
		protocol: newProtocol(h, dht),
		notifee:  notifee,
	}
}

// Discover establishes a connection to a set of bootstrap peers
// that we're using to connect to the DHT. It tries to find
func (d *Discoverer) Discover(chanID int) error {
	if err := d.ServiceStarted(); err != nil {
		return err
	}
	defer d.ServiceStopped()

	for {
		did := d.did.DiscoveryID(chanID)
		log.Debugln("DHT - Discovering", did)
		cID, err := d.did.ContentID(did)
		if err != nil {
			return err
		}

		// Find new provider with a timeout, so the discovery ID is renewed if necessary.
		ctx, cancel := context.WithTimeout(d.ServiceContext(), provideTimeout)
		for pi := range d.dht.FindProvidersAsync(ctx, cID, 100) {
			log.Debugln("DHT - Found peer ", pi.ID)
			if isRoutable(pi) {
				go d.notifee.HandlePeerFound(pi)
			}
		}
		log.Debugln("DHT - Discovering", did, " done.")

		// cannot defer cancel in this for loop
		cancel()

		select {
		case <-d.SigShutdown():
			return nil
		default:
		}
	}
}

func (d *Discoverer) SetOffset(offset time.Duration) *Discoverer {
	d.did.SetOffset(offset)
	return d
}

func (d *Discoverer) Shutdown() {
	d.Service.Shutdown()
}

func isRoutable(pi peer.AddrInfo) bool {
	return len(pi.Addrs) > 0
}
