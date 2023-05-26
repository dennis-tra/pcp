package host

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

func (h *Host) handleKeyMsg(msg tea.KeyMsg) (*Host, tea.Cmd) {
	h.logEntry().WithField("key", msg.String()).Infoln("Key msg", msg.String())
	switch msg.String() {
	case "v":
		h.Verbose = !h.Verbose
	case "ctrl+c":
		return h, Shutdown
	}
	return h, nil
}

func (h *Host) handleLocalAddressesUpdated(evt event.EvtLocalAddressesUpdated) (*Host, tea.Cmd) {
	h.logEntry().WithFields(logrus.Fields{
		"diff":  evt.Diffs,
		"count": len(evt.Current),
	}).Infoln("Local addresses updated")

	maddrs := make([]ma.Multiaddr, len(evt.Current))
	for i, update := range evt.Current {
		maddrs[i] = update.Address
	}

	h.populateAddrs(maddrs)

	return h, h.watchEvents
}

func (h *Host) handleNATDeviceTypeChanged(evt event.EvtNATDeviceTypeChanged) (*Host, tea.Cmd) {
	h.logEntry().WithFields(logrus.Fields{
		"type":     evt.NatDeviceType.String(),
		"protocol": evt.TransportProtocol.String(),
	}).Infoln("NAT Device Type Changed")

	switch evt.TransportProtocol {
	case network.NATTransportUDP:
		h.NATTypeUDP = evt.NatDeviceType
	case network.NATTransportTCP:
		h.NATTypeTCP = evt.NatDeviceType
	}

	return h, h.watchEvents
}

func (h *Host) handleLocalReachabilityChanged(evt event.EvtLocalReachabilityChanged) (*Host, tea.Cmd) {
	h.logEntry().WithFields(logrus.Fields{
		"reachability": evt.Reachability.String(),
	}).Infoln("Local Reachability changed")

	h.Reachability = evt.Reachability

	return h, h.watchEvents
}

func (h *Host) handleHolePunchEvent(evt *holepunch.Event) (*Host, tea.Cmd) {
	h.logEntry().WithFields(logrus.Fields{
		"peerID": evt.Peer.String()[:16],
	}).Infoln("Hole Punch Event")

	return h, nil
}
