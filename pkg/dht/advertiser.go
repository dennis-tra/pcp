package dht

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/host"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/internal/wrap"
)

const (
	// Timeout for pushing our data to the DHT.
	provideTimeout = 30 * time.Second
)

// Advertiser is responsible for writing and renewing the DHT entry.
type Advertiser struct {
	*protocol
}

// NewAdvertiser creates a new Advertiser.
func NewAdvertiser(h host.Host, dht wrap.IpfsDHT) *Advertiser {
	return &Advertiser{newProtocol(h, dht)}
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

	if err := a.bootstrap(); err != nil {
		a.setError(err)
		return
	}

	if err := a.checkNetwork(); err != nil {
		a.setError(err)
		return
	}

	did := a.did.DiscoveryID(chanID)

	a.setStage(StageProviding)
	for {
		select {
		case <-a.SigShutdown():
			log.Debugln("DHT - Advertising", did, " done - shutdown signal")
			a.setStage(StageStopped)
			return
		default:
		}

		err := a.provide(a.ServiceContext(), did)
		if err != nil {
			a.setStage(StageRetrying)
		} else {
			a.setStage(StageProvided)
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

// Shutdown stops the advertisement mechanics.
func (a *Advertiser) Shutdown() {
	a.Service.Shutdown()
}
