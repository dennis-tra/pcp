package dht

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/internal/wrap"
)

// Timeout for pushing our data to the DHT.
var (
	provideTimeout = 30 * time.Second
)

// Advertiser is responsible for writing and renewing the DHT entry.
type Advertiser struct {
	*protocol[*AdvertiseState]
}

// NewAdvertiser creates a new Advertiser.
func NewAdvertiser(h host.Host, dht wrap.IpfsDHT) *Advertiser {
	a := &Advertiser{
		protocol: newProtocol[*AdvertiseState](h, dht),
	}

	a.protocol.state = &AdvertiseState{
		Stage:        StageIdle,
		Reachability: network.ReachabilityUnknown,
		NATTypeTCP:   network.NATDeviceTypeUnknown,
		NATTypeUDP:   network.NATDeviceTypeUnknown,
	}

	// populate the address slices
	a.state.populateAddrs(h.Addrs())

	return a
}

// Advertise establishes a connection to a set of bootstrap peers
// that we're using to connect to the DHT. Then it puts the
// discovery identifier into the DHT (timeout 1 minute - provideTimeout)
// and renews the identifier when a new time slot is reached.
// Time slots are used as a kind of sharding for peer discovery.
// pcp nodes says: "Hey, you can find me with channel ID 123". Then,
// one hour later another, completely unrelated pcp node comes along and says
// "Hey, you can find me with channel ID 123". A peer searching for 123
// would find the new and the stale entry. To avoid finding the stale entry
// we use the current time truncated to 5 minute intervals (TruncateDuration).
// When pcp is advertising its own channel-id + time slot it can happen that
// it rolls over to the next time slot. Then, pcp just advertises the new
// time slot as well. It can still be found with the old one.
func (a *Advertiser) Advertise(chanID int) {
	if err := a.ServiceStarted(); err != nil {
		a.setError(err)
		return
	}
	defer a.ServiceStopped()

	// bootstrap
	a.setStage(StageBootstrapping)
	err := a.bootstrap()
	if err != nil {
		a.setError(err)
		return
	}

AnalyzeNetwork:
	a.setStage(StageAnalyzingNetwork)
	err = a.analyzeNetwork()
	if err != nil {
		a.setError(err)
		return
	}

	go func() {
		if err := a.watchNetwork(); err != nil {
			a.Shutdown()
		}
	}()

	ticker := time.NewTicker(tickInterval)
	a.setStage(StageProviding)
	for {

		canDirectConn, err := a.isDirectConnectivityPossible()
		if !canDirectConn && err == nil {
			log.Debugln("Analyzing network again because we lost direct connectivity possibility")
			goto AnalyzeNetwork
		}

		// Always regenerate discovery ID
		did := a.did.DiscoveryID(chanID)

		err = a.provide(a.ServiceContext(), did)
		if err != nil {
			a.setStage(StageRetrying)
		} else {
			a.setStage(StageProvided)
		}

		select {
		case <-a.SigShutdown():
			log.Debugln("DHT - Advertising", did, " done - shutdown signal")
			a.setStage(StageStopped)
			return
		case <-ticker.C:
		}
	}
}

// the context requires a timeout; it determines how long the DHT looks for
// closest peers to the key/CID before it goes on to provide the record to them.
// Not setting a timeout here will make the DHT wander forever.
func (a *Advertiser) provide(ctx context.Context, did string) error {
	log.Debugln("DHT - Advertising", did)
	defer log.Debugln("DHT - Advertising", did, "done")
	cID, err := a.did.ContentID(did)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, provideTimeout)
	defer cancel()

	return a.dht.Provide(ctx, cID, true)
}

// analyzeNetwork subscribes to a couple of libp2p events that fire after certain network conditions where determined.
func (a *Advertiser) analyzeNetwork() error {
	log.Debugln("Start analyzing network")
	defer log.Debugln("Stop analyzing network")

	evtTypes := []interface{}{
		new(event.EvtLocalReachabilityChanged),
		new(event.EvtNATDeviceTypeChanged),
		new(event.EvtLocalAddressesUpdated),
	}
	sub, err := a.EventBus().Subscribe(evtTypes)
	if err != nil {
		return fmt.Errorf("subscribe to libp2p eventbus: %w", err)
	}
	defer sub.Close()

	for {
		canDirectConn, err := a.isDirectConnectivityPossible()
		if canDirectConn {
			return nil
		} else if err != nil {
			return err
		}

		if err := a.consumeEvent(sub); err != nil {
			return fmt.Errorf("consume event: %w", err)
		}
	}
}

func (a *Advertiser) isDirectConnectivityPossible() (bool, error) {
	if a.state.Reachability == network.ReachabilityPrivate && a.state.NATTypeUDP == network.NATDeviceTypeSymmetric && a.state.NATTypeTCP == network.NATDeviceTypeSymmetric {
		return false, fmt.Errorf("private network with symmetric NAT")
	}

	// we have public reachability, we're good to go with the DHT
	if a.state.Reachability == network.ReachabilityPublic && len(a.state.PublicAddrs) > 0 {
		return true, nil
	}

	// we are in a private network, but have at least one cone NAT and at least one relay address
	if a.state.Reachability == network.ReachabilityPrivate && (a.state.NATTypeUDP == network.NATDeviceTypeCone || a.state.NATTypeTCP == network.NATDeviceTypeCone) && len(a.state.RelayAddrs) > 0 {
		return true, nil
	}

	return false, nil
}

func (a *Advertiser) watchNetwork() error {
	log.Debugln("Start watching network")
	defer log.Debugln("Stop watching network")

	evtTypes := []interface{}{
		new(event.EvtLocalReachabilityChanged),
		new(event.EvtNATDeviceTypeChanged),
		new(event.EvtLocalAddressesUpdated),
	}
	sub, err := a.EventBus().Subscribe(evtTypes)
	if err != nil {
		return fmt.Errorf("subscribe to libp2p eventbus: %w", err)
	}
	defer sub.Close()

	for {
		if err := a.consumeEvent(sub); err != nil {
			return fmt.Errorf("consume event: %w", err)
		}
	}
}

func (a *Advertiser) consumeEvent(sub event.Subscription) error {
	var (
		e    interface{}
		more bool
	)

	select {
	case <-a.ServiceContext().Done():
		return a.ServiceContext().Err()
	case e, more = <-sub.Out():
		if !more {
			return fmt.Errorf("subscription closed")
		}
	}

	a.stateLk.Lock()
	switch evt := e.(type) {
	case event.EvtLocalReachabilityChanged:
		a.state.Reachability = evt.Reachability
	case event.EvtNATDeviceTypeChanged:
		switch evt.TransportProtocol {
		case network.NATTransportUDP:
			a.state.NATTypeUDP = evt.NatDeviceType
		case network.NATTransportTCP:
			a.state.NATTypeTCP = evt.NatDeviceType
		}
	case event.EvtLocalAddressesUpdated:
		maddrs := make([]ma.Multiaddr, len(evt.Current))
		for i, update := range evt.Current {
			maddrs[i] = update.Address
		}
		a.state.populateAddrs(maddrs)
	}
	a.stateLk.Unlock()

	return nil
}
