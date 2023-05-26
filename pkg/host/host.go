package host

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/mdns"
	"github.com/dennis-tra/pcp/pkg/words"
)

var log = logrus.WithField("comp", "host")

// Host encapsulates the logic that's common for the receiving
// and sending side of the file transfer.
type Host struct {
	host.Host

	// give host protocol capabilities
	*PakeProtocol
	//*PushProtocol
	//*TransferProtocol

	ctx     context.Context
	program *tea.Program
	Verbose bool

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

	MDNS *mdns.MDNS
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
}

// New creates a new, fully initialized host with the given options.
func New(ctx context.Context, program *tea.Program, wrds []string, opts ...libp2p.Option) (*Host, error) {
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
	opts = append(opts,
		// libp2p.UserAgent("pcp/"+c.App.Version),
		libp2p.ResourceManager(rm),
		libp2p.EnableHolePunching(holepunch.WithTracer(&holePunchTracer{program: program})),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			ipfsDHT, err = kaddht.New(ctx, h, kaddht.EnableOptimisticProvide())
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
	pcpHost := &Host{
		ctx:          ctx,
		Host:         h,
		IpfsDHT:      ipfsDHT,
		program:      program,
		Words:        wrds,
		MDNS:         mdns.New(ctx, h, program, chanID),
		DHT:          dht.New(ctx, h, ipfsDHT, chanID),
		evtSub:       evtSub,
		NATManager:   nat,
		PeerStates:   map[peer.ID]PeerState{},
		PakeProtocol: NewPakeProtocol(ctx, h, program, wrds),
		// PushProtocol: NewPushProtocol(host)
		// TransferProtocol: NewTransferProtocol(host)
	}

	pcpHost.Network().Notify(pcpHost)

	// Extract addresses from host AFTER we have subscribed to the address
	// change events. Otherwise, there could have been a race condition.
	pcpHost.populateAddrs(pcpHost.Addrs())

	return pcpHost, nil
}

func (h *Host) Init() tea.Cmd {
	h.RegisterKeyExchangeHandler()

	return tea.Batch(
		h.watchSignals,
		h.watchEvents,
		h.MDNS.Init(),
		h.DHT.Init(),
	)
}

type ShutdownMsg struct{}

func Shutdown() tea.Msg {
	return ShutdownMsg{}
}

func (h *Host) logEntry() *logrus.Entry {
	return log.WithFields(logrus.Fields{
		"comp": "host",
	})
}

func (h *Host) Update(msg tea.Msg) (*Host, tea.Cmd) {
	h.logEntry().WithField("type", fmt.Sprintf("%T", msg)).Tracef("handle message: %T\n", msg)

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case connectedMsg:
		h.connections += 1
	case disconnectedMsg:
		h.connections -= 1
	case pakeOnKeyExchange:
		h.PeerStates[msg.Conn().RemotePeer()] = PeerStateAuthenticating
	case tea.KeyMsg:
		h, cmd = h.handleKeyMsg(msg)
		cmds = append(cmds, cmd)
	case *holepunch.Event:
		h, cmd = h.handleHolePunchEvent(msg)
		cmds = append(cmds, cmd)
	case event.EvtLocalAddressesUpdated:
		h, cmd = h.handleLocalAddressesUpdated(msg)
		cmds = append(cmds, cmd)
	case event.EvtNATDeviceTypeChanged:
		h, cmd = h.handleNATDeviceTypeChanged(msg)
		cmds = append(cmds, cmd)
	case event.EvtLocalReachabilityChanged:
		h, cmd = h.handleLocalReachabilityChanged(msg)
		cmds = append(cmds, cmd)
	}

	h.MDNS, cmd = h.MDNS.Update(msg)
	cmds = append(cmds, cmd)

	h.DHT, cmd = h.DHT.Update(msg)
	cmds = append(cmds, cmd)

	h.PakeProtocol, cmd = h.PakeProtocol.Update(msg)
	cmds = append(cmds, cmd)

	return h, tea.Batch(cmds...)
}

func (h *Host) View() string {
	if !h.Verbose {
		return ""
	}

	out := ""
	out += fmt.Sprintf("PeerID:        %s\n", h.ID())
	out += fmt.Sprintf("DHT:           %s\n", h.DHT.State.String())
	out += fmt.Sprintf("mDNS:          %s\n", h.MDNS.State.String())
	out += fmt.Sprintf("Reachability:  %s\n", h.Reachability.String())
	out += fmt.Sprintf("Connections:   %d\n", h.connections)
	out += fmt.Sprintf("NAT (udp/tcp): %s / %s\n", h.NATTypeUDP.String(), h.NATTypeTCP.String())
	out += fmt.Sprintf("Addresses:\n")
	out += fmt.Sprintf("  Private:     %d\n", len(h.PrivateAddrs))
	out += fmt.Sprintf("  Public:      %d\n", len(h.PublicAddrs))
	out += fmt.Sprintf("  Relay:       %d\n", len(h.RelayAddrs))

	var mappings []string
	for _, pm := range h.portMappings() {
		mappings = append(mappings, pm.String())
	}
	sort.Strings(mappings)
	if len(mappings) > 0 {
		out += fmt.Sprintf("Port Mappings:\n")
		for _, mapping := range mappings {
			out += fmt.Sprintf("  %s\n", mapping)
		}
	}

	return out
}

func (h *Host) StartKeyExchange(ctx context.Context, remotePeer peer.ID) tea.Cmd {
	h.PeerStates[remotePeer] = PeerStateAuthenticating
	return h.PakeProtocol.StartKeyExchange(ctx, remotePeer)
}

func (h *Host) watchSignals() tea.Msg {
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
	case <-h.ctx.Done():
		return nil
	}
}

func (h *Host) watchEvents() tea.Msg {
	select {
	case evt := <-h.evtSub.Out():
		return evt
	case <-h.ctx.Done():
		return nil
	}
}

func (h *Host) IsDirectConnectivityPossible() (bool, error) {
	if h.Reachability == network.ReachabilityPrivate && h.NATTypeUDP == network.NATDeviceTypeSymmetric && h.NATTypeTCP == network.NATDeviceTypeSymmetric {
		return false, fmt.Errorf("private network with symmetric NAT")
	}

	// we have public reachability, we're good to go with the DHT
	if h.Reachability == network.ReachabilityPublic && len(h.PublicAddrs) > 0 {
		return true, nil
	}

	// we are in a private network, but have at least one cone NAT and at least one relay address
	if h.Reachability == network.ReachabilityPrivate && (h.NATTypeUDP == network.NATDeviceTypeCone || h.NATTypeTCP == network.NATDeviceTypeCone) && len(h.RelayAddrs) > 0 {
		return true, nil
	}

	return false, nil
}

func (h *Host) populateAddrs(addrs []ma.Multiaddr) {
	h.PublicAddrs = []ma.Multiaddr{}
	h.PrivateAddrs = []ma.Multiaddr{}
	h.RelayAddrs = []ma.Multiaddr{}
	for _, addr := range addrs {
		if isRelayedMaddr(addr) { // needs to come before IsPublic, because relay addrs are also public addrs
			h.RelayAddrs = append(h.RelayAddrs, addr)
		} else if manet.IsPublicAddr(addr) {
			h.PublicAddrs = append(h.PublicAddrs, addr)
		} else if manet.IsPrivateAddr(addr) {
			h.PrivateAddrs = append(h.PrivateAddrs, addr)
		}
	}
}

func isRelayedMaddr(maddr ma.Multiaddr) bool {
	_, err := maddr.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

type holePunchTracer struct {
	program *tea.Program
}

func (h *holePunchTracer) Trace(evt *holepunch.Event) {
	h.program.Send(evt)
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

func (h *Host) Listen(n network.Network, multiaddr ma.Multiaddr) {}

func (h *Host) ListenClose(n network.Network, multiaddr ma.Multiaddr) {}

func (h *Host) Connected(n network.Network, conn network.Conn) {
	h.program.Send(connectedMsg{net: n, conn: conn})
}

func (h *Host) Disconnected(n network.Network, conn network.Conn) {
	h.program.Send(disconnectedMsg{net: n, conn: conn})
}
