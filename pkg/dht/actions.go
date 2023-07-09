package dht

import (
	"context"
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/discovery"
)

// connectToBootstrapper connects to a set of bootstrap nodes to connect to the DHT.
func (d *DHT) connectToBootstrapper() tea.Msg {
	log.Infoln("Start bootstrapping")

	var peers []peer.AddrInfo
	for _, maddrStr := range config.Global.BootstrapPeers.Value() {

		maddr, err := ma.NewMultiaddr(maddrStr)
		if err != nil {
			log.WithError(err).WithField("maddr", maddrStr).Warnln("Couldn't parse multiaddress")
			continue
		}

		pi, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.WithError(err).WithField("maddr", maddr).Warningln("Couldn't craft peer addr info")
			continue
		}

		peers = append(peers, *pi)
	}

	peerCount := len(peers)
	if peerCount < 4 {
		return bootstrapResultMsg{err: fmt.Errorf("too few Bootstrap peers configured (min %d)", ConnThreshold)}
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
			logEntry := log.WithField("peerID", pi.ID.String()[:16])
			logEntry.Debugln("Connecting bootstrap peer")
			err := d.Connect(d.ctx, pi)
			if err != nil {
				logEntry.WithError(err).Warnln("Failed connecting to bootstrap peer")
			} else {
				logEntry.Infoln("Connected to bootstrap peer!")
			}
			errChan <- err
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
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		default:
			return bootstrapResultMsg{err: errs}
		}
	}

	return bootstrapResultMsg{err: nil}
}

func (d *DHT) provide(ctx context.Context, offset time.Duration) tea.Cmd {
	return func() tea.Msg {
		did := discovery.NewID(offset)

		logEntry := log.WithField("did", did.DiscoveryID(d.chanID))
		logEntry.Debugln("Start providing")
		defer logEntry.Debugln("Done providing")

		cID, err := did.ContentID(did.DiscoveryID(d.chanID))
		if err != nil {
			return advertiseResult{
				offset: offset,
				err:    err,
				fatal:  true,
			}
		}

		return advertiseResult{
			offset: offset,
			err:    d.dht.Provide(ctx, cID, true),
			fatal:  false,
		}
	}
}

func (d *DHT) lookup(ctx context.Context, offset time.Duration) tea.Cmd {
	return func() tea.Msg {
		did := discovery.NewID(offset)

		logEntry := log.WithField("did", did.DiscoveryID(d.chanID))
		logEntry.Debugln("Start lookup")
		defer logEntry.Debugln("Done lookup")

		cID, err := did.ContentID(did.DiscoveryID(d.chanID))
		if err != nil {
			return PeerMsg{
				offset: offset,
				err:    err,
				fatal:  true,
			}
		}

		// Find new provider with a timeout, so the discovery ID is renewed if necessary.
		ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
		defer cancel()
		for pi := range d.dht.FindProvidersAsync(ctx, cID, 0) {
			logEntry.Debugln("DHT - Found peer ", pi.ID)
			if len(pi.Addrs) > 0 {
				return PeerMsg{
					Peer:   pi,
					offset: offset,
				}
			}
		}

		return PeerMsg{
			err: fmt.Errorf("not found"),
		}
	}
}
