package dht

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/host"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/internal/wrap"
)

var (
	// Timeout for pushing our data to the DHT.
	provideTimeout = time.Minute

	// Interval between two checks whether we know our public
	// IP address. This can take time until e.g. the identify
	// protocol has determined one for us.
	pubAddrInter = 50 * time.Millisecond
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
// it rolls over to the next time slot. Than pcp just advertises the new time slot
// as well. It can still be found with the old one.
func (a *Advertiser) Advertise(chanID int) error {
	if err := a.Bootstrap(); err != nil {
		return err
	}

	if err := a.ServiceStarted(); err != nil {
		return err
	}
	defer a.ServiceStopped()

	for {
		// Only advertise in the DHT if we have a public addr.
		if !a.HasPublicAddr() {
			select {
			case <-a.SigShutdown():
				return nil
			case <-time.After(pubAddrInter):
				continue
			}
		}

		// Newly advertise as soon as the new time slot is reached
		ctx, cancel := context.WithTimeout(a.ServiceContext(), a.DurNextSlot())
		err := a.provide(ctx, a.DiscoveryID(chanID))
		cancel()
		if err == context.Canceled {
			break
		} else if err != nil && err != context.DeadlineExceeded {
			log.Warningf("Error providing: %s\n", err)
		}
	}

	return nil
}

// HasPublicAddr returns true if there is at least one public
// address associated with the current node - aka we got at
// least three confirmations from peers through the identify
// protocol.
func (a *Advertiser) HasPublicAddr() bool {
	for _, addr := range a.Addrs() {
		if wrapmanet.IsPublicAddr(addr) {
			return true
		}
	}
	return false
}

// Shutdown stops the advertise mechanics.
func (a *Advertiser) Shutdown() {
	a.Service.Shutdown()
}

// the context requires a timeout; it determines how long the DHT looks for
// closest peers to the key/CID before it goes on to provide the record to them.
// Not setting a timeout here will make the DHT wander forever.
func (a *Advertiser) provide(ctx context.Context, id string) error {
	cID, err := strToCid(id)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, provideTimeout)
	defer cancel()
	return a.dht.Provide(ctx, cID, true)
}
