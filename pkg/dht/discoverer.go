package dht

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/dennis-tra/pcp/pkg/discovery"
)

const (
	// Timeout for looking up our data in the DHT
	lookupTimeout = 10 * time.Second
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
func (d *Discoverer) Discover(chanID int) {
	if err := d.ServiceStarted(); err != nil {
		d.setError(err)
		return
	}
	defer d.ServiceStopped()

	if err := d.bootstrap(); err != nil {
		d.setError(err)
		return
	}

	d.setStage(StageLookup)
	for {
		did := d.did.DiscoveryID(chanID)
		log.Debugln("DHT - Discovering", did)
		cID, err := d.did.ContentID(did)
		if err != nil {
			d.setError(err)
			return
		}

		// Find new provider with a timeout, so the discovery ID is renewed if necessary.
		ctx, cancel := context.WithTimeout(d.ServiceContext(), lookupTimeout)
		for pi := range d.dht.FindProvidersAsync(ctx, cID, 0) {
			log.Debugln("DHT - Found peer ", pi.ID)
			if len(pi.Addrs) > 0 {
				go d.notifee.HandlePeerFound(pi)
			}
		}
		log.Debugln("DHT - Discovering", did, " done.")

		// cannot defer cancel in this for loop
		cancel()

		select {
		case <-d.SigShutdown():
			log.Debugln("DHT - Discovering", did, " done - shutdown signal")
			d.setStage(StageStopped)
			return
		default:
			d.setStage(StageRetrying)
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
