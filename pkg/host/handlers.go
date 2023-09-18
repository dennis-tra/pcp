package host

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dennis-tra/pcp/pkg/mdns"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (*Model, tea.Cmd) {
	m.logEntry().WithField("key", msg.String()).Infoln("Key msg", msg.String())

	var cmd tea.Cmd
	switch msg.String() {
	case "v":
		m.Verbose = !m.Verbose
	case "m":
		switch m.MDNS.State {
		case mdns.StateIdle, mdns.StateError, mdns.StateStopped:
			m.MDNS, cmd = m.MDNS.Start()
		case mdns.StateStarted:
			m.MDNS = m.MDNS.Stop()
		}
	case "ctrl+c":
		return m, Shutdown
	}

	return m, cmd
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

// HolePunchState encapsulates all information about the current state
// that we are in during the hole punch exchange. The notifyFailed
// channel is closed when the hole punch failed and listeners
// were registered.
type HolePunchState struct {
	Stage    HolePunchStage
	Attempts int
	Err      error
}

type HolePunchStage uint8

const (
	HolePunchStageUnknown = iota
	HolePunchStageStarted
	HolePunchStageSucceeded
	HolePunchStageFailed
)

func (m *Model) handleHolePunchEvent(evt *holepunch.Event) (*Model, tea.Cmd) {
	m.logEntry().WithFields(logrus.Fields{
		"peerID": evt.Peer.String()[:16],
	}).Infoln("Hole Punch Event")

	//_, ok := m.PeerStates[evt.Remote]
	//if !ok {
	//	// Check if the remote has initiated the hole punch
	//	isIncoming := false
	//	for _, conn := range m.Network().ConnsToPeer(evt.Remote) {
	//		if conn.Stat().Direction == network.DirInbound {
	//			isIncoming = true
	//			break
	//		}
	//	}
	//
	//	if !isIncoming {
	//		return m, nil
	//	}
	//}

	if _, found := m.hpStates[evt.Remote]; !found {
		m.hpStates[evt.Remote] = &HolePunchState{
			Stage: HolePunchStageUnknown,
		}
	}

	prefix := fmt.Sprintf("HolePunch %s (%s): ", evt.Remote.String()[:16], evt.Type)
	switch e := evt.Evt.(type) {
	case *holepunch.StartHolePunchEvt:
		log.Debugf(prefix+"rtt=%s\n", e.RTT)
		m.hpStates[evt.Remote].Stage = HolePunchStageStarted
	case *holepunch.DirectDialEvt:
		log.Debugf(prefix+"success=%s err=%v time=%s\n", strconv.FormatBool(e.Success), e.Error, e.EllapsedTime)
		if e.Success {
			m.hpStates[evt.Remote].Stage = HolePunchStageSucceeded
		}
	case *holepunch.HolePunchAttemptEvt:
		log.Debugf(prefix+"attempt=%d\n", e.Attempt)
		m.hpStates[evt.Remote].Attempts += 1
	case *holepunch.ProtocolErrorEvt:
		log.Debugf(prefix+"err=%v\n", e.Error)
		m.hpStates[evt.Remote].Stage = HolePunchStageFailed
		m.hpStates[evt.Remote].Err = fmt.Errorf(e.Error)
	case *holepunch.EndHolePunchEvt:
		log.Debugf(prefix+"success=%s err=%v time=%s\n", strconv.FormatBool(e.Success), e.Error, e.EllapsedTime)
		if e.Success {
			m.hpStates[evt.Remote].Stage = HolePunchStageSucceeded
			m.hpStates[evt.Remote].Err = nil
		} else {
			m.hpStates[evt.Remote].Stage = HolePunchStageFailed
			m.hpStates[evt.Remote].Err = fmt.Errorf(e.Error)
		}
	default:
		panic("unexpected hole punch event")
	}

	if m.state == HostStateWaitingForDirectConn {
		if state, found := m.hpStates[m.authedPeer]; found && state.Stage == HolePunchStageFailed && state.Attempts >= 3 {
			// TODO: some user feedback
			return m, Shutdown
		}
	}

	return m, nil
}
