package dht

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/dennis-tra/pcp/pkg/discovery"
)

// connectToBootstrapper connects to a set of bootstrap nodes to connect to the DHT.
func (d *DHT) connectToBootstrapper(pi peer.AddrInfo) tea.Cmd {
	return func() tea.Msg {
		logEntry := log.WithField("peerID", pi.ID.String()[:16])
		logEntry.Debugln("Connecting bootstrap peer")
		err := d.Connect(d.ctx, pi)
		if err != nil {
			logEntry.WithError(err).Warnln("Failed connecting to bootstrap peer")
			return bootstrapResultMsg{err: err}
		} else {
			logEntry.Infoln("Connected to bootstrap peer!")
			return bootstrapResultMsg{err: nil}
		}
	}
}

func (d *DHT) provide(ctx context.Context, offset time.Duration) tea.Cmd {
	return func() tea.Msg {
		did := discovery.NewID(d.chanID).SetRole(d.role).SetOffset(offset)

		logEntry := log.WithField("did", did.DiscoveryID())
		logEntry.Debugln("Start providing")
		defer logEntry.Debugln("Done providing")

		cID, err := did.ContentID()
		if err != nil {
			return advertiseResultMsg{
				offset: offset,
				err:    err,
				fatal:  true,
			}
		}

		return advertiseResultMsg{
			offset: offset,
			err:    d.dht.Provide(ctx, cID, true),
			fatal:  false,
		}
	}
}

func (d *DHT) lookup(ctx context.Context, offset time.Duration) tea.Cmd {
	return func() tea.Msg {
		did := discovery.NewID(d.chanID).SetRole(d.role.Opposite()).SetOffset(offset)

		logEntry := log.WithField("did", did.DiscoveryID())
		logEntry.Debugln("Start lookup")
		defer logEntry.Debugln("Done lookup")

		cID, err := did.ContentID()
		if err != nil {
			return PeerMsg{
				offset: offset,
				Err:    err,
				fatal:  true,
			}
		}

		// Find new provider with a timeout, so the discovery ID is renewed if necessary.
		ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
		defer cancel()

		for pi := range d.dht.FindProvidersAsync(ctx, cID, 0) {
			pi := pi

			logEntry.Debugln("Found peer ", pi.ID)
			if len(pi.Addrs) > 0 {
				return PeerMsg{
					Peer:   pi,
					offset: offset,
				}
			}
		}

		return PeerMsg{
			Err: fmt.Errorf("not found"),
		}
	}
}
