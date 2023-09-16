package dht

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/discovery"
)

type (
	bootstrapResultMsg struct {
		oid int
		err error
	}
	advertiseResultMsg struct {
		oid int
		err error
	}
	PeerMsg struct { // discoverResult
		oid  int
		Peer peer.AddrInfo
	}
	lookupDone struct {
		oid int
	}
)

func (m *Model) Bootstrap() (*Model, tea.Cmd) {
	var cmds []tea.Cmd
	for _, bp := range config.Global.BoostrapAddrInfos() {
		m.BootstrapsPending += 1
		ctx, oid := m.newOperation(0, bootstrapTimeout)
		cmds = append(cmds, m.connectToBootstrapper(ctx, bp, oid))
	}

	m.State = StateBootstrapping

	return m, tea.Batch(cmds...)
}

func (m *Model) Start() (*Model, tea.Cmd) {
	var cmds []tea.Cmd

	cmds = append(cmds, m.provide(0))
	cmds = append(cmds, m.lookup(0))
	cmds = append(cmds, m.lookup(-discovery.TruncateDuration))

	m.State = StateActive

	return m, tea.Batch(cmds...)
}

func (m *Model) StartNoProvides() (*Model, tea.Cmd) {
	var cmds []tea.Cmd

	cmds = append(cmds, m.lookup(0))
	cmds = append(cmds, m.lookup(-discovery.TruncateDuration))

	m.State = StateActive

	return m, tea.Batch(cmds...)
}

func (m *Model) Stop() (*Model, tea.Cmd) {
	m.State = StateStopping

	for _, ref := range m.ops {
		ref.cancel()
	}

	return m, nil
}

// connectToBootstrapper connects to a set of bootstrap nodes to connect to the DHT.
func (m *Model) connectToBootstrapper(ctx context.Context, pi peer.AddrInfo, oid int) tea.Cmd {
	return func() tea.Msg {
		logEntry := log.WithField("peerID", pi.ID.String()[:16])
		logEntry.Debugln("Connecting bootstrap peer")

		if err := m.host.Connect(ctx, pi); err != nil {
			logEntry.WithError(err).Warnln("Failed connecting to bootstrap peer")
			return bootstrapResultMsg{oid: oid, err: err}
		}

		logEntry.Infoln("Connected to bootstrap peer!")
		return bootstrapResultMsg{oid: oid, err: nil}
	}
}

func (m *Model) provide(offset time.Duration) tea.Cmd {
	m.PendingProvides += 1
	ctx, oid := m.newOperation(offset, lookupTimeout)
	return func() tea.Msg {
		did := discovery.NewID(m.chanID).SetRole(m.role).SetOffset(offset)

		logEntry := log.WithField("did", did.DiscoveryID())
		logEntry.Debugln("Start providing")
		defer logEntry.Debugln("Done providing")

		return advertiseResultMsg{
			oid: oid,
			err: m.dht.Provide(ctx, did.ContentID(), true),
		}
	}
}

func (m *Model) lookup(offset time.Duration) tea.Cmd {
	m.PendingLookups += 1
	ctx, oid := m.newOperation(offset, lookupTimeout)
	return func() tea.Msg {
		did := discovery.NewID(m.chanID).SetRole(m.role.Opposite()).SetOffset(offset)

		logEntry := log.WithField("did", did.DiscoveryID())
		logEntry.Debugln("Start lookup")
		defer logEntry.Debugln("Done lookup")

		for pi := range m.dht.FindProvidersAsync(ctx, did.ContentID(), 0) {
			pi := pi

			logEntry.Debugln("Found peer ", pi.ID)
			if len(pi.Addrs) > 0 {
				m.sender.Send(PeerMsg{
					oid:  oid,
					Peer: pi,
				})
			}
		}

		return lookupDone{
			oid: oid,
		}
	}
}
