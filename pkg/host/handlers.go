package host

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/mdns"
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

func (m *Model) handlePeerFound(addrInfo peer.AddrInfo) (*Model, tea.Cmd) {
	peerState, found := m.PeerStates[addrInfo.ID]
	if found {
		switch peerState {
		case PeerStateNotConnected:
			m.PeerStates[addrInfo.ID] = PeerStateConnecting
			return m, m.connect(addrInfo)
		case PeerStateConnecting:
			log.Debugln("Ignoring discovered peer as we're already trying to connect", addrInfo.ID)
		case PeerStateConnected:
			log.Debugln("Ignoring discovered peer because as we're already connected", addrInfo.ID)
		case PeerStateAuthenticating:
			log.Debugln("Ignoring discovered peer because as we're in midst of authenticating each other", addrInfo.ID)
		case PeerStateAuthenticated:
			log.Debugln("Ignoring discovered peer as it's already authenticated", addrInfo.ID)
		case PeerStateFailedConnecting:
			log.Debugln("We tried to connect previously but couldn't establish a connection, try again", addrInfo.ID)
			m.PeerStates[addrInfo.ID] = PeerStateConnecting
			return m, m.connect(addrInfo)
		case PeerStateFailedAuthentication:
			log.Debugln("We tried to connect previously but the node didn't pass authentication -> skipping", addrInfo.ID)
		}
	} else {
		m.PeerStates[addrInfo.ID] = PeerStateConnecting
		return m, m.connect(addrInfo)
	}
	return m, nil
}

func (m *Model) handlePeerAuthenticated(pid peer.ID) (*Model, tea.Cmd) {
	// check if we already have an authenticated peer
	if m.authedPeer != "" {
		log.WithField("authenticated", m.authedPeer.String()).WithField("new", pid.String()).Debugln("already connected and authenticated with another peer")
		if err := m.Host.Network().ClosePeer(pid); err != nil {
			log.WithError(err).Debugln("error closing newly authenticated peer")
		}

		m.PeerStates[pid] = PeerStateFailedAuthentication

		return m, nil
	}

	log.WithField("peer", pid).Infoln("Found peer!")

	m.state = HostStateWaitingForDirectConn
	m.PeerStates[pid] = PeerStateAuthenticated
	m.authedPeer = pid

	m.debugLogAuthenticatedPeer(pid)
	m.AuthProt.UnregisterKeyExchangeHandler()

	if m.role == discovery.RoleReceiver {
		m.PushProt.RegisterPushRequestHandler(pid)
	}

	m.DHT = m.DHT.Stop()
	m.MDNS = m.MDNS.Stop()

	for _, p := range m.Host.Peerstore().Peers() {
		if p == pid {
			continue
		}

		if err := m.Host.Network().ClosePeer(p); err != nil {
			log.WithError(err).WithField("peer", p.String()).Warnln("Failed closing connection to peer")
		}
	}

	direct := 0
	indirect := 0
	for _, conn := range m.Network().ConnsToPeer(pid) {
		if isRelayAddress(conn.RemoteMultiaddr()) {
			indirect += 1
		} else {
			direct += 1
		}
	}

	if direct > 0 {
		m.PeerStates[pid] = PeerStateAuthenticated
		switch m.role {
		case discovery.RoleReceiver:
			// TODO: configure timeout
			return m, nil
		case discovery.RoleSender:
			m.state = HostStateWaitingForAcceptance
			return m, m.PushProt.SendPushRequest(pid, "", 2, false)
		}
	}

	if indirect == 0 {
		m.state = HostStateLostConnection
		return m, Shutdown
	}

	return m, func() tea.Msg {
		// TODO: configure timeout
		return "timeout"
	}
}
