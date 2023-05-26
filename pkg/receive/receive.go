package receive

import (
	"context"
	"fmt"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/discovery"
	pcphost "github.com/dennis-tra/pcp/pkg/host"
	"github.com/dennis-tra/pcp/pkg/mdns"
)

var log = logrus.WithField("comp", "receive")

type Model struct {
	ctx               context.Context
	host              *pcphost.Host
	program           *tea.Program
	relayFinderActive bool
}

func NewState(ctx context.Context, program *tea.Program, words []string) (*Model, error) {
	if !config.Global.DHT && !config.Global.MDNS {
		return nil, fmt.Errorf("either the DHT or mDNS discovery mechanism need to be active")
	}

	log.Infoln("Random words:", strings.Join(words, "-"))

	// Start the libp2p node
	h, err := pcphost.New(ctx, program, words)
	if err != nil {
		return nil, err
	}

	model := &Model{
		ctx:     ctx,
		host:    h,
		program: program,
	}

	return model, nil
}

func (m *Model) Init() tea.Cmd {
	log.Traceln("tea init")
	return m.host.Init()
}

type connectMsg struct {
	id  peer.ID
	err error
}

func (m *Model) connect(pi peer.AddrInfo) tea.Cmd {
	return func() tea.Msg {
		log.Debugln("Connecting to peer:", pi.ID)
		err := m.host.Connect(m.ctx, peer.AddrInfo(pi))
		if err != nil {
			log.Debugln("Error connecting to peer:", pi.ID, err)
		}
		return connectMsg{
			id:  pi.ID,
			err: err,
		}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	log.WithField("type", fmt.Sprintf("%T", msg)).Tracef("handle message: %T\n", msg)

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	m.host, cmd = m.host.Update(msg)
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case dht.PeerMsg:
	case mdns.PeerMsg:

		peerState, found := m.host.PeerStates[msg.ID]
		if found {
			switch peerState {
			case pcphost.PeerStateNotConnected:
				m.host.PeerStates[msg.ID] = pcphost.PeerStateConnecting
				cmds = append(cmds, m.connect(peer.AddrInfo(msg)))
			case pcphost.PeerStateConnecting:
				log.Debugln("Ignoring discovered peer as we're already trying to connect", msg.ID)
			case pcphost.PeerStateConnected:
				log.Debugln("Ignoring discovered peer because as we're already connected", msg.ID)
			case pcphost.PeerStateAuthenticated:
				log.Debugln("Ignoring discovered peer as it's already authenticated", msg.ID)
			case pcphost.PeerStateFailedConnecting:
				log.Debugln("We tried to connect previously but couldn't establish a connection, try again", msg.ID)
				m.host.PeerStates[msg.ID] = pcphost.PeerStateConnecting
				cmds = append(cmds, m.connect(peer.AddrInfo(msg)))
			case pcphost.PeerStateFailedAuthentication:
				log.Debugln("We tried to connect previously but the node didn't pass authentication -> skipping", msg.ID)
			}
		} else {
			m.host.PeerStates[msg.ID] = pcphost.PeerStateConnecting
			cmds = append(cmds, m.connect(peer.AddrInfo(msg)))
		}
	case connectMsg:
		if msg.err == nil {
			m.host.PeerStates[msg.id] = pcphost.PeerStateConnected
			cmds = append(cmds, m.host.StartKeyExchange(m.ctx, msg.id))
		} else {
			m.host.PeerStates[msg.id] = pcphost.PeerStateFailedConnecting
		}

	case syscall.Signal:
		cmds = append(cmds, m.handleSignal(msg))
	case tea.KeyMsg:
		cmds = append(cmds, m.handleKeyMsg(msg))
	case pcphost.ShutdownMsg:
		return m, tea.Quit
	}

	switch m.host.MDNS.State {
	case mdns.StateIdle:
		if config.Global.MDNS && len(m.host.PrivateAddrs) > 0 {
			m.host.MDNS, cmd = m.host.MDNS.Start(0, discovery.TruncateDuration)
			cmds = append(cmds, cmd)
		}
	}

	switch m.host.DHT.State {
	case dht.StateIdle:
		if config.Global.DHT {
			m.host.DHT, cmd = m.host.DHT.Bootstrap()
			cmds = append(cmds, cmd)
		}
	case dht.StateBootstrapping:
	case dht.StateBootstrapped:
		m.host.DHT, cmd = m.host.DHT.Discover(0, discovery.TruncateDuration)
		cmds = append(cmds, cmd)
	}

	//	// TODO: if dht + mdns are in error -> stop

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	out := ""

	code := strings.Join(m.host.Words, "-")
	out += fmt.Sprintf("Looking for peer %s...\n", code)

	style := lipgloss.NewStyle().Bold(true)

	out += m.host.View()

	if m.host.Verbose {
		relayFinderState := "inactive"
		if m.relayFinderActive {
			relayFinderState = "active"
		}
		out += fmt.Sprintf("Relay finder:  %s\n", relayFinderState)
	}

	internet := m.host.DHT.View()
	if m.host.Reachability == network.ReachabilityPrivate && m.relayFinderActive && m.host.DHT.State == dht.StateBootstrapped {
		internet = "finding signaling peers"
	}

	out += fmt.Sprintf("%s %s\t %s %s\n", style.Render("Local Network:"), m.host.MDNS.View(), style.Render("Internet:"), internet)

	out += m.host.ViewPeerStates()

	return out
}

func (m *Model) handleSignal(sig syscall.Signal) tea.Cmd {
	log.WithField("sig", sig.String()).Infoln("received signal")
	return tea.Quit
}

func (m *Model) handleKeyMsg(keyMsg tea.KeyMsg) tea.Cmd {
	switch keyMsg.Type {
	}
	return nil
}
