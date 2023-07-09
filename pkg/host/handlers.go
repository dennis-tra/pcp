package host

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (*Model, tea.Cmd) {
	m.logEntry().WithField("key", msg.String()).Infoln("Key msg", msg.String())
	switch msg.String() {
	case "v":
		m.Verbose = !m.Verbose
	case "ctrl+c":
		return m, Shutdown
	}
	return m, nil
}

func (m *Model) handleLocalAddressesUpdated(evt event.EvtLocalAddressesUpdated) (*Model, tea.Cmd) {
	m.logEntry().WithFields(logrus.Fields{
		"diff":  evt.Diffs,
		"count": len(evt.Current),
	}).Infoln("Local addresses updated")

	maddrs := make([]ma.Multiaddr, len(evt.Current))
	for i, update := range evt.Current {
		maddrs[i] = update.Address
	}

	return m.populateAddrs(maddrs), m.watchEvents
}

func (m *Model) handleNATDeviceTypeChanged(evt event.EvtNATDeviceTypeChanged) (*Model, tea.Cmd) {
	m.logEntry().WithFields(logrus.Fields{
		"type":     evt.NatDeviceType.String(),
		"protocol": evt.TransportProtocol.String(),
	}).Infoln("NAT Device Type Changed")

	switch evt.TransportProtocol {
	case network.NATTransportUDP:
		m.NATTypeUDP = evt.NatDeviceType
	case network.NATTransportTCP:
		m.NATTypeTCP = evt.NatDeviceType
	}

	return m, m.watchEvents
}

func (m *Model) handleLocalReachabilityChanged(evt event.EvtLocalReachabilityChanged) (*Model, tea.Cmd) {
	m.logEntry().WithFields(logrus.Fields{
		"reachability": evt.Reachability.String(),
	}).Infoln("Local Reachability changed")

	m.Reachability = evt.Reachability

	return m, m.watchEvents
}

func (m *Model) handleHolePunchEvent(evt *holepunch.Event) (*Model, tea.Cmd) {
	m.logEntry().WithFields(logrus.Fields{
		"peerID": evt.Peer.String()[:16],
	}).Infoln("Hole Punch Event")

	return m, nil
}
