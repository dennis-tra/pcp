package host

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/mdns"
	"github.com/dennis-tra/pcp/pkg/words"
)

var log = logrus.WithField("comp", "host")

// Model encapsulates the logic that's common for the receiving
// and sending side of the file transfer.
type Model struct {
	host.Host

	// give host protocol capabilities
	*AuthProtocol
	//*PushProtocol
	//*TransferProtocol

	ctx     context.Context
	sender  tea.Sender
	Verbose bool

	relayFinderActive bool

	// keeps track of hole punching states of particular peers.
	// The hpAllowList is populated by the receiving side of
	// the file transfer after it has discovered the peer via
	// mDNS or in the DHT. If a peer is in the hpAllowList the
	// hpStates map will track the hole punching state for
	// that particular peer. The sending side doesn't work
	// with that map and instead tracks all **incoming** hole
	// punches. I've observed that the sending side does try
	// to hole punch peers it finds in the DHT (while advertising).
	// These are hole punches we're not interested in.
	// hpStates    map[peer.ID]*HolePunchState
	hpAllowList map[peer.ID]struct{}

	// DHT is an accessor that is needed in the DHT discoverer/advertiser.
	IpfsDHT    *kaddht.IpfsDHT
	NATManager basichost.NATManager

	MDNS *mdns.Model
	DHT  *dht.DHT

	NATTypeUDP   network.NATDeviceType
	NATTypeTCP   network.NATDeviceType
	Reachability network.Reachability
	PublicAddrs  []ma.Multiaddr
	PrivateAddrs []ma.Multiaddr
	RelayAddrs   []ma.Multiaddr
	PeerStates   map[peer.ID]PeerState

	connections int

	Words  []string
	evtSub event.Subscription

	spinner spinner.Model
}

// New creates a new, fully initialized host with the given options.
func New(ctx context.Context, sender tea.Sender, role discovery.Role, wrds []string, opts ...libp2p.Option) (*Model, error) {
	ints, err := words.ToInts(wrds)
	if err != nil {
		return nil, fmt.Errorf("words to ints: %w", err)
	}

	// Configure the resource manager to not limit anything
	limiter := rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits)
	rm, err := rcmgr.NewResourceManager(limiter)
	if err != nil {
		return nil, fmt.Errorf("new resource manager: %w", err)
	}

	var (
		ipfsDHT *kaddht.IpfsDHT
		nat     basichost.NATManager
	)

	rf := &relayFinder{sender: sender}
	opts = append(opts,
		libp2p.UserAgent("pcp/"+config.Global.Version),
		libp2p.ResourceManager(rm),
		libp2p.EnableAutoRelayWithPeerSource(
			rf.autoRelayPeerSource,
			autorelay.WithMetricsTracer(rf),
			autorelay.WithBootDelay(0),
			autorelay.WithMinCandidates(1),
		),
		libp2p.EnableHolePunching(holepunch.WithTracer(&holePunchTracer{sender: sender})),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			ipfsDHT, err = kaddht.New(ctx, h, kaddht.EnableOptimisticProvide(), kaddht.Datastore(datastore.NewNullDatastore()))
			return ipfsDHT, err
		}),
		libp2p.NATManager(func(network network.Network) basichost.NATManager {
			nat = basichost.NewNATManager(network)
			return nat
		}),
	)

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new libp2p host: %w", err)
	}

	evtSub, err := h.EventBus().Subscribe([]interface{}{
		new(event.EvtLocalAddressesUpdated),
		new(event.EvtNATDeviceTypeChanged),
		new(event.EvtLocalReachabilityChanged),
	})
	if err != nil {
		return nil, fmt.Errorf("event bus subscription: %w", err)
	}

	log.WithField("peerID", h.ID().String()).Infoln("Initialized libp2p host")

	chanID := ints[0]
	model := &Model{
		ctx:          ctx,
		Host:         h,
		IpfsDHT:      ipfsDHT,
		sender:       sender,
		Words:        wrds,
		MDNS:         mdns.New(h, sender, role, chanID),
		DHT:          dht.New(ctx, h, ipfsDHT, role, chanID),
		evtSub:       evtSub,
		NATManager:   nat,
		PeerStates:   map[peer.ID]PeerState{},
		AuthProtocol: NewAuthProtocol(ctx, h, sender, role, wrds),
		// PushProtocol: NewPushProtocol(host)
		// TransferProtocol: NewTransferProtocol(host)
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
	}

	// properly init relayFinder
	rf.dht = ipfsDHT
	rf.host = h

	model.Network().Notify(model)

	// Extract addresses from host AFTER we have subscribed to the address
	// change events. Otherwise, there could have been a race condition.
	model = model.populateAddrs(model.Addrs())

	return model, nil
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.watchSignals,
		m.watchEvents,
		m.MDNS.Init(),
		m.DHT.Init(),
		m.AuthProtocol.Init(),
		m.spinner.Tick,
	)
}

type ShutdownMsg struct{}

func Shutdown() tea.Msg {
	return ShutdownMsg{}
}

func (m *Model) logEntry() *logrus.Entry {
	return log.WithFields(logrus.Fields{
		"comp": "host",
	})
}

func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	m.logEntry().WithField("type", fmt.Sprintf("%T", msg)).Tracef("handle message: %T\n", msg)

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case connectedMsg:
		m.connections += 1
	case disconnectedMsg:
		m.connections -= 1
	case dht.PeerMsg:
		if msg.Err == nil {
			m, cmd = m.HandlePeerFound(msg.Peer)
			cmds = append(cmds, cmd)
		}
	case mdns.PeerMsg:
		m, cmd = m.HandlePeerFound(peer.AddrInfo(msg))
		cmds = append(cmds, cmd)
	case authOnKeyExchange:
		m.PeerStates[msg.stream.Conn().RemotePeer()] = PeerStateAuthenticating
	case authMsg[[]byte]:
		m.PeerStates[msg.peerID] = PeerStateAuthenticated
		if m.DHT.State != dht.StateIdle && m.DHT.State != dht.StateError {
			m.DHT, cmd = m.DHT.StopWithReason(nil)
		}
		if m.MDNS.State != mdns.StateIdle && m.MDNS.State != mdns.StateError {
			m.MDNS, cmd = m.MDNS.StopWithReason(nil)
		}
	case authMsg[error]:
		m.PeerStates[msg.peerID] = PeerStateFailedAuthentication
	case relayFinderStatus:
		m.relayFinderActive = msg.isActive
	case tea.KeyMsg:
		m, cmd = m.handleKeyMsg(msg)
		cmds = append(cmds, cmd)
	case *holepunch.Event:
		m, cmd = m.handleHolePunchEvent(msg)
		cmds = append(cmds, cmd)
	case event.EvtLocalAddressesUpdated:
		m, cmd = m.handleLocalAddressesUpdated(msg)
		cmds = append(cmds, cmd)
	case event.EvtNATDeviceTypeChanged:
		m, cmd = m.handleNATDeviceTypeChanged(msg)
		cmds = append(cmds, cmd)
	case event.EvtLocalReachabilityChanged:
		m, cmd = m.handleLocalReachabilityChanged(msg)
		cmds = append(cmds, cmd)
	case syscall.Signal:
		cmds = append(cmds, m.handleSignal(msg))
	}

	m.MDNS, cmd = m.MDNS.Update(msg)
	cmds = append(cmds, cmd)

	m.DHT, cmd = m.DHT.Update(msg)
	cmds = append(cmds, cmd)

	m.AuthProtocol, cmd = m.AuthProtocol.Update(msg)
	cmds = append(cmds, cmd)

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	out := ""
	if m.Verbose {

		out += fmt.Sprintf("PeerID:        %s\n", m.ID())
		switch m.DHT.State {
		case dht.StateBootstrapping:
			out += fmt.Sprintf("DHT:           %s (pending %d, success %d, errors %d)\n", m.DHT.State.String(), m.DHT.BootstrapsPending, m.DHT.BootstrapsSuccesses, len(m.DHT.BootstrapsErrs))
		case dht.StateError:
			out += fmt.Sprintf("DHT:           %s (%s)\n", m.DHT.State.String(), m.DHT.Err)
		default:
			out += fmt.Sprintf("DHT:           %s\n", m.DHT.State.String())
		}
		out += fmt.Sprintf("mDNS:          %s\n", m.MDNS.State.String())
		out += fmt.Sprintf("Reachability:  %s\n", m.Reachability.String())
		out += fmt.Sprintf("Connections:   %d\n", m.connections)
		out += fmt.Sprintf("NAT (udp/tcp): %s / %s\n", m.NATTypeUDP.String(), m.NATTypeTCP.String())
		out += fmt.Sprintf("Addresses:\n")
		out += fmt.Sprintf("  Private:     %d\n", len(m.PrivateAddrs))
		out += fmt.Sprintf("  Public:      %d\n", len(m.PublicAddrs))
		out += fmt.Sprintf("  Relay:       %d\n", len(m.RelayAddrs))

		var mappings []string
		for _, pm := range m.portMappings() {
			mappings = append(mappings, pm.String())
		}
		sort.Strings(mappings)
		if len(mappings) > 0 {
			out += fmt.Sprintf("Port Mappings:\n")
			for _, mapping := range mappings {
				out += fmt.Sprintf("  %s\n", mapping)
			}
		}

		relayFinderState := "inactive"
		if m.relayFinderActive {
			relayFinderState = "active"
		}
		out += fmt.Sprintf("Relay finder:  %s\n", relayFinderState)
	}

	style := lipgloss.NewStyle().Bold(true)

	local := m.MDNS.View()
	internet := m.DHT.View()
	if m.Reachability == network.ReachabilityPrivate && m.relayFinderActive && m.DHT.State == dht.StateBootstrapped {
		internet = m.spinner.View() + "(finding signaling peers)"
	}

	out += fmt.Sprintf("%s %s\t %s %s\n", style.Render("Local Network:"), local, style.Render("Internet:"), internet)

	out += m.ViewPeerStates()

	return out
}

func (m *Model) StartKeyExchange(ctx context.Context, remotePeer peer.ID) tea.Cmd {
	if state, found := m.PeerStates[remotePeer]; found {
		switch state {
		case PeerStateConnected:
		case PeerStateFailedConnecting:
		default:
			return nil
		}
	}

	if remotePeer.String() == "" {
		panic("hfdhhfggh")
	}
	m.PeerStates[remotePeer] = PeerStateAuthenticating
	return m.AuthProtocol.StartKeyExchange(ctx, remotePeer)
}

type PeerConnectMsg struct {
	ID  peer.ID
	Err error
}

func (m *Model) connect(pi peer.AddrInfo) tea.Cmd {
	return func() tea.Msg {
		log.Debugln("Connecting to peer:", pi.ID)
		return PeerConnectMsg{
			ID:  pi.ID,
			Err: m.Connect(m.ctx, pi),
		}
	}
}

func (m *Model) HandlePeerFound(pi peer.AddrInfo) (*Model, tea.Cmd) {
	peerState, found := m.PeerStates[pi.ID]
	if found {
		switch peerState {
		case PeerStateNotConnected:
			m.PeerStates[pi.ID] = PeerStateConnecting
			return m, m.connect(pi)
		case PeerStateConnecting:
			log.Debugln("Ignoring discovered peer as we're already trying to connect", pi.ID)
		case PeerStateConnected:
			log.Debugln("Ignoring discovered peer because as we're already connected", pi.ID)
		case PeerStateAuthenticating:
			log.Debugln("Ignoring discovered peer because as we're in midst of authenticating each other", pi.ID)
		case PeerStateAuthenticated:
			log.Debugln("Ignoring discovered peer as it's already authenticated", pi.ID)
		case PeerStateFailedConnecting:
			log.Debugln("We tried to connect previously but couldn't establish a connection, try again", pi.ID)
			m.PeerStates[pi.ID] = PeerStateConnecting
			return m, m.connect(pi)
		case PeerStateFailedAuthentication:
			log.Debugln("We tried to connect previously but the node didn't pass authentication -> skipping", pi.ID)
		}
	} else {
		m.PeerStates[pi.ID] = PeerStateConnecting
		return m, m.connect(pi)
	}
	return m, nil
}

func (m *Model) watchSignals() tea.Msg {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	defer func() {
		signal.Stop(sigs)
		log.Debugln("stopped watching signals")
	}()

	log.Debugln("Watch OS signals")
	select {
	case sig := <-sigs:
		log.WithField("sig", sig.String()).Debugln("saw signal")
		return sig
	case <-m.ctx.Done():
		return nil
	}
}

func (m *Model) handleSignal(sig syscall.Signal) tea.Cmd {
	log.WithField("sig", sig.String()).Infoln("received signal")
	return func() tea.Msg {
		return ShutdownMsg{}
	}
}

func (m *Model) watchEvents() tea.Msg {
	select {
	case evt := <-m.evtSub.Out():
		return evt
	case <-m.ctx.Done():
		return nil
	}
}

func (m *Model) IsDirectConnectivityPossible() (bool, error) {
	if m.Reachability == network.ReachabilityPrivate && m.NATTypeUDP == network.NATDeviceTypeSymmetric && m.NATTypeTCP == network.NATDeviceTypeSymmetric {
		return false, fmt.Errorf("private network with symmetric NAT")
	}

	// we have public reachability, we're good to go with the DHT
	if m.Reachability == network.ReachabilityPublic && len(m.PublicAddrs) > 0 {
		return true, nil
	}

	// we are in a private network, but have at least one cone NAT and at least one relay address
	if m.Reachability == network.ReachabilityPrivate && (m.NATTypeUDP == network.NATDeviceTypeCone || m.NATTypeTCP == network.NATDeviceTypeCone) && len(m.RelayAddrs) > 0 {
		return true, nil
	}

	return false, nil
}

func (m *Model) populateAddrs(addrs []ma.Multiaddr) *Model {
	m.PublicAddrs = []ma.Multiaddr{}
	m.PrivateAddrs = []ma.Multiaddr{}
	m.RelayAddrs = []ma.Multiaddr{}
	for _, addr := range addrs {
		if isRelayedMaddr(addr) { // needs to come before IsPublic, because relay addrs are also public addrs
			m.RelayAddrs = append(m.RelayAddrs, addr)
		} else if manet.IsPublicAddr(addr) {
			m.PublicAddrs = append(m.PublicAddrs, addr)
		} else if manet.IsPrivateAddr(addr) {
			m.PrivateAddrs = append(m.PrivateAddrs, addr)
		}
	}
	return m
}

func isRelayedMaddr(maddr ma.Multiaddr) bool {
	_, err := maddr.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

type holePunchTracer struct {
	sender tea.Sender
}

func (h *holePunchTracer) Trace(evt *holepunch.Event) {
	h.sender.Send(evt)
}

type (
	connectedMsg struct {
		net  network.Network
		conn network.Conn
	}

	disconnectedMsg struct {
		net  network.Network
		conn network.Conn
	}
)

func (m *Model) Connected(n network.Network, conn network.Conn) {
	m.sender.Send(connectedMsg{net: n, conn: conn})
}

func (m *Model) Disconnected(n network.Network, conn network.Conn) {
	m.sender.Send(disconnectedMsg{net: n, conn: conn})
}

func (m *Model) Listen(n network.Network, multiaddr ma.Multiaddr) {}

func (m *Model) ListenClose(n network.Network, multiaddr ma.Multiaddr) {}

type relayFinder struct {
	host   host.Host
	dht    *kaddht.IpfsDHT
	sender tea.Sender
}

// autoRelayPeerSource is a function that queries the DHT for a random peer ID with CPL 0.
// The found peers are used as candidates for circuit relay v2 peers.
func (r *relayFinder) autoRelayPeerSource(ctx context.Context, num int) <-chan peer.AddrInfo {
	log.Debugln("Looking for auto relay peers...")

	out := make(chan peer.AddrInfo)

	go func() {
		defer log.Debugln("Looking for auto relay peers... Done!")
		defer close(out)

		peerID, err := r.dht.RoutingTable().GenRandPeerID(0)
		if err != nil {
			log.Debugln("error generating random peer ID:", err.Error())
			return
		}

		closestPeers, err := r.dht.GetClosestPeers(ctx, peerID.String())
		if err != nil {
			return
		}

		maxLen := len(closestPeers)
		if maxLen > num {
			maxLen = num
		}

		for i := 0; i < maxLen; i++ {
			p := closestPeers[i]

			addrs := r.host.Peerstore().Addrs(p)
			if len(addrs) == 0 {
				continue
			}

			log.Debugln("Found auto relay peer:", p.String()[:16])
			select {
			case out <- peer.AddrInfo{ID: p, Addrs: addrs}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

type relayFinderStatus struct {
	isActive bool
}

func (r *relayFinder) RelayFinderStatus(isActive bool) {
	r.sender.Send(relayFinderStatus{isActive: isActive})
}

func (r *relayFinder) ReservationEnded(cnt int) {}

func (r *relayFinder) ReservationOpened(cnt int) {}

func (r *relayFinder) ReservationRequestFinished(isRefresh bool, err error) {}

func (r *relayFinder) RelayAddressCount(i int) {}

func (r *relayFinder) RelayAddressUpdated() {}

func (r *relayFinder) CandidateChecked(supportsCircuitV2 bool) {}

func (r *relayFinder) CandidateAdded(cnt int) {}

func (r *relayFinder) CandidateRemoved(cnt int) {}

func (r *relayFinder) CandidateLoopState(state autorelay.CandidateLoopState) {}

func (r *relayFinder) ScheduledWorkUpdated(scheduledWork *autorelay.ScheduledWorkTimes) {}

func (r *relayFinder) DesiredReservations(i int) {}
