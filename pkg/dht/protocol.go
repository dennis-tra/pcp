package dht

import (
	"fmt"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	mh "github.com/multiformats/go-multihash"

	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/dennis-tra/pcp/pkg/service"
)

// These wrapped top level functions are here for testing purposes.
var (
	wrapDHT   wrap.DHTer   = wrap.DHT{}
	wraptime  wrap.Timer   = wrap.Time{}
	wrapmanet wrap.Maneter = wrap.Manet{}
)

var (
	// ConnThreshold represents the minimum number of bootstrap peers we need a connection to.
	ConnThreshold = 3

	// TruncateDuration represents the time slot to which the current time is truncated.
	TruncateDuration = 5 * time.Minute

	// bootstrap holds the sync.Onces for each host, so that bootstrap is called for each host
	// only once.
	bootstrap = map[peer.ID]*sync.Once{} // may need locking in theory?
)

// protocol encapsulates the logic for discovering peers
// through providing it in the IPFS DHT.
type protocol struct {
	host.Host

	// Service holds an abstraction of a long running
	// service that is started and stopped externally.
	*service.Service
	dht wrap.IpfsDHT

	offset time.Duration
}

func newProtocol(h host.Host, dht wrap.IpfsDHT) *protocol {
	bootstrap[h.ID()] = &sync.Once{}
	return &protocol{Host: h, dht: dht, Service: service.New("DHT")}
}

// Bootstrap connects to a set of bootstrap nodes to connect
// to the DHT.
func (p *protocol) Bootstrap() (err error) {
	// The receiving peer looks for the current and previous time slot. So it would call
	// bootstrap twice. Here we're limiting it to only one call.
	once := bootstrap[p.ID()]
	once.Do(func() {
		peers := wrapDHT.GetDefaultBootstrapPeerAddrInfos()
		peerCount := len(peers)
		if peerCount == 0 {
			err = fmt.Errorf("no bootstrap peers configured")
			return
		}

		// Asynchronously connect to all bootstrap peers and send
		// potential errors to a channel. This channel is used
		// to capture the errors and check if we have established
		// enough connections. An error group (errgroup) cannot
		// be used here as it exits as soon as an error is thrown
		// in one of the Go-Routines.
		var wg sync.WaitGroup
		errChan := make(chan error, peerCount)
		for _, bp := range peers {
			wg.Add(1)
			go func(pi peer.AddrInfo) {
				defer wg.Done()
				errChan <- p.Connect(p.ServiceContext(), pi)
			}(bp)
		}

		// Close error channel after all connection attempts are done
		// to signal the for-loop below to stop.
		go func() {
			wg.Wait()
			close(errChan)
		}()

		// Reading the error channel and collect errors.
		errs := ErrConnThresholdNotReached{BootstrapErrs: []error{}}
		for {
			err, ok := <-errChan
			if !ok {
				// channel was closed.
				break
			} else if err != nil {
				errs.BootstrapErrs = append(errs.BootstrapErrs, err)
			}
		}

		// If we could not establish enough connections return an error
		if peerCount-len(errs.BootstrapErrs) < ConnThreshold {
			err = errs
		}
	})
	return
}

// TimeSlotStart returns the time when the current time slot started.f
func (p *protocol) TimeSlotStart() time.Time {
	return p.refTime().Truncate(TruncateDuration)
}

// refTime returns the reference time to calculate the time slot from.
func (p *protocol) refTime() time.Time {
	return wraptime.Now().Add(p.offset)
}

// DiscoveryID returns the string, that we use to advertise
// via mDNS and the DHT. See chanID above for more information.
// Using UnixNano for testing.
func (p *protocol) DiscoveryID(chanID int) string {
	return fmt.Sprintf("/pcp/%d/%d", p.TimeSlotStart().UnixNano(), chanID)
}

// strToCid hashes the given string (SHA256) and produces a CID from that hash.
func strToCid(str string) (cid.Cid, error) {
	h, err := mh.Sum([]byte(str), mh.SHA2_256, -1)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(cid.Raw, h), nil
}
