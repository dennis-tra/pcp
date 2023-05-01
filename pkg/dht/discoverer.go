package dht

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	ma "github.com/multiformats/go-multiaddr"

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
	*protocol[*DiscoverState]

	notifee discovery.Notifee
}

// NewDiscoverer creates a new Discoverer.
func NewDiscoverer(h host.Host, dht wrap.IpfsDHT, notifee discovery.Notifee) *Discoverer {
	d := &Discoverer{
		protocol: newProtocol[*DiscoverState](h, dht),
		notifee:  notifee,
	}

	d.protocol.state = &DiscoverState{
		Stage: StageIdle,
	}

	return d
}

// Discover establishes a connection to a set of bootstrap peers
// that we're using to connect to the DHT. It tries to find
func (d *Discoverer) Discover(chanID int) {
	if err := d.ServiceStarted(); err != nil {
		d.setError(err)
		return
	}
	defer d.ServiceStopped()

	d.setStage(StageBootstrapping)
	err := d.bootstrap()
	if err != nil {
		d.setError(err)
		return
	}

	d.setStage(StageWaitingForPublicAddrs)
	err = d.waitPublicAddresses()
	if err != nil {
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
		cancel()

		log.Debugln("DHT - Discovering", did, "done")

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

// waitPublicAddresses blocks until we've found public addresses
func (d *Discoverer) waitPublicAddresses() error {
	evtTypes := []interface{}{
		new(event.EvtLocalAddressesUpdated),
	}
	sub, err := d.EventBus().Subscribe(evtTypes)
	if err != nil {
		return fmt.Errorf("subscribe to libp2p eventbus: %w", err)
	}
	defer sub.Close()

	for {
		var e interface{}

		select {
		case <-d.ServiceContext().Done():
			return d.ServiceContext().Err()
		case e = <-sub.Out():
		}

		d.stateLk.Lock()
		switch evt := e.(type) {
		case event.EvtLocalAddressesUpdated:
			maddrs := make([]ma.Multiaddr, len(evt.Current))
			for i, update := range evt.Current {
				maddrs[i] = update.Address
			}
			d.state.populateAddrs(maddrs)
		}
		d.stateLk.Unlock()

		if len(d.state.PublicAddrs) > 0 {
			return nil
		}
	}
}
