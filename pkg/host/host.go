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
	"github.com/dennis-tra/pcp/pkg/events"
	"github.com/dennis-tra/pcp/pkg/mdns"
	"github.com/dennis-tra/pcp/pkg/words"
)

var log = logrus.WithField("comp", "host")

// Host encapsulates the logic that's common for the receiving
// and sending side of the file transfer.
type Host struct {
	ctx context.Context

	host.Host

	verbose bool

	// give host protocol capabilities
	*PakeProtocol
	//*PushProtocol
	//*TransferProtocol

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

	ChanID int
	Words  []string

	hpEmitter   *events.Emitter[*holepunch.Event]
	connEmitter *events.Emitter[network.Conn]
	ebSub       events.Subscription[interface{}]
}

type Option func()

// New creates a new, fully initialized host with the given options.
func New(ctx context.Context, wrds []string, opts ...libp2p.Option) (*Host, error) {
	ints, err := words.ToInts(wrds)
	if err != nil {
		return nil, fmt.Errorf("words to ints: %w", err)
	}

	pcpHost := &Host{
		ctx: ctx,
		// hpStates:    map[peer.ID]*HolePunchState{},
		Words:       wrds,
		ChanID:      ints[0],
		hpEmitter:   events.NewEmitter[*holepunch.Event](),
		connEmitter: events.NewEmitter[network.Conn](),
	}

	// Configure the resource manager to not limit anything
	limiter := rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits)
	rm, err := rcmgr.NewResourceManager(limiter)
	if err != nil {
		return nil, fmt.Errorf("new resource manager: %w", err)
	}

	opts = append(opts,
		// libp2p.UserAgent("pcp/"+c.App.Version),
		libp2p.ResourceManager(rm),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			pcpHost.IpfsDHT, err = kaddht.New(ctx, h, kaddht.EnableOptimisticProvide())
			return pcpHost.IpfsDHT, err
		}),
		libp2p.EnableHolePunching(holepunch.WithTracer(pcpHost)),
		libp2p.NATManager(func(network network.Network) basichost.NATManager {
			pcpHost.NATManager = basichost.NewNATManager(network)
			return pcpHost.NATManager
		}),
	)

	pcpHost.Host, err = libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new libp2p host: %w", err)
	}

	pcpHost.PakeProtocol = NewPakeProtocol(ctx, pcpHost.Host, wrds)
	// host.PushProtocol = NewPushProtocol(host)
	// host.TransferProtocol = NewTransferProtocol(host)

	pcpHost.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(net network.Network, conn network.Conn) {
			pcpHost.connEmitter.Emit(conn)
		},
	})

	evtTypes := []interface{}{
		new(event.EvtLocalAddressesUpdated),
		new(event.EvtNATDeviceTypeChanged),
		new(event.EvtLocalReachabilityChanged),
	}

	sub, err := pcpHost.EventBus().Subscribe(evtTypes)
	if err != nil {
		return nil, fmt.Errorf("event bus subscription: %w", err)
	}
	pcpHost.ebSub = events.NewSubscription(sub.Out())

	log.WithField("peerID", pcpHost.ID().String()).Infoln("Initialized libp2p host")

	// Extract addresses from host AFTER we have subscribed to the address
	// change events. Otherwise, there could have been a race condition.
	pcpHost.populateAddrs(pcpHost.Addrs())

	pcpHost.MDNS = mdns.New(ctx, pcpHost)
	pcpHost.DHT = dht.New(ctx, pcpHost, pcpHost.IpfsDHT)

	return pcpHost, nil
}

func (h *Host) Init() tea.Cmd {
	return tea.Batch(
		h.watchSignals,
		h.ebSub.Subscribe,
		h.hpEmitter.Subscribe,
		h.connEmitter.Subscribe,
		h.RegisterKeyExchangeHandler(),
		h.MDNS.Init(),
		h.DHT.Init(),
	)
}

type ShutdownMsg struct{}

func Shutdown() tea.Msg {
	return ShutdownMsg{}
}

func (h *Host) Update(msg tea.Msg) (*Host, tea.Cmd) {
	log.WithFields(logrus.Fields{
		"comp": "host",
		"type": fmt.Sprintf("%T", msg),
	}).Tracef("handle message: %T\n", msg)

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		log.Infoln(msg.Type.String())
		switch msg.String() {
		case "v":
			h.verbose = !h.verbose
		case "ctrl+c":
			return h, Shutdown
		}

	case events.Msg[*holepunch.Event]:
		cmds = append(cmds, msg.Listen)
	case events.Msg[network.Conn]:
		cmds = append(cmds, msg.Listen)
	case events.Msg[interface{}]:
		switch evt := msg.Value.(type) {
		case event.EvtLocalAddressesUpdated:
			maddrs := make([]ma.Multiaddr, len(evt.Current))
			for i, update := range evt.Current {
				maddrs[i] = update.Address
			}
			h.populateAddrs(maddrs)
			cmds = append(cmds, msg.Listen)
		case event.EvtNATDeviceTypeChanged:
			switch evt.TransportProtocol {
			case network.NATTransportUDP:
				h.NATTypeUDP = evt.NatDeviceType
			case network.NATTransportTCP:
				h.NATTypeTCP = evt.NatDeviceType
			}
			cmds = append(cmds, msg.Listen)
		case event.EvtLocalReachabilityChanged:
			h.Reachability = evt.Reachability
			cmds = append(cmds, msg.Listen)
		}
		cmds = append(cmds, msg.Listen)
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
	if !h.verbose {
		return ""
	}

	out := ""
	out += fmt.Sprintf("DHT:           %s\n", h.DHT.State.String())
	out += fmt.Sprintf("mDNS:          %s\n", h.MDNS.State.String())
	out += fmt.Sprintf("Reachability:  %s\n", h.Reachability.String())
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

func (h *Host) Trace(evt *holepunch.Event) {
	h.hpEmitter.Emit(evt)
}

func isRelayedMaddr(maddr ma.Multiaddr) bool {
	_, err := maddr.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}
