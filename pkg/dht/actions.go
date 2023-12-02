package dht

import (
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
	for _, bp := range config.Global.BootstrapPeers {
		m.BootstrapsPending += 1
		cmds = append(cmds, m.connectToBootstrapper(bp))
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

func (m *Model) StartNoProvide() (*Model, tea.Cmd) {
	var cmds []tea.Cmd

	cmds = append(cmds, m.lookup(0))
	cmds = append(cmds, m.lookup(-discovery.TruncateDuration))

	m.State = StateActive

	return m, tea.Batch(cmds...)
}

func (m *Model) StartProvide() (*Model, tea.Cmd) {
	return m, m.provide(0)
}

func (m *Model) Stop() *Model {
	switch m.State {
	case StateIdle, StateStopped, StateError:
		return m
	default:
		// pass
	}

	if len(m.ops) == 0 {
		m.State = StateStopped
		return m
	}

	m.State = StateStopping

	for _, ref := range m.ops {
		ref.cancel()
	}

	if err := m.dht.Close(); err != nil {
		log.WithError(err).Warnln("Failed closing kademlia DHT")
	}

	return m
}

// connectToBootstrapper connects to a set of bootstrap nodes to connect to the DHT.
func (m *Model) connectToBootstrapper(pi peer.AddrInfo) tea.Cmd {
	ctx, oid := m.newOperation(0, bootstrapTimeout)
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
