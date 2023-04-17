package dht

import (
	"fmt"
	"sync"

	"github.com/dennis-tra/pcp/pkg/discovery"

	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/dennis-tra/pcp/pkg/service"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
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

	// bootstrap holds the sync.Onces for each host, so that bootstrap is called for each host
	// only once.
	bootstrap = map[peer.ID]*sync.Once{} // may need locking in theory?
)

// protocol encapsulates the logic for discovering peers
// through providing it in the IPFS DHT.
type protocol struct {
	host.Host

	// Service holds an abstraction of a long-running
	// service that is started and stopped externally.
	service.Service
	dht wrap.IpfsDHT
	did discovery.ID
}

func newProtocol(h host.Host, dht wrap.IpfsDHT) *protocol {
	bootstrap[h.ID()] = &sync.Once{}
	return &protocol{
		Host:    h,
		dht:     dht,
		Service: service.New("DHT"),
		did:     discovery.ID{},
	}
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
