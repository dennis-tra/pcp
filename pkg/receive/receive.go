package receive

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/libp2p/go-libp2p/core/network"
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
	host              *pcphost.Model
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

	m.host.RegisterKeyExchangeHandler(pcphost.PakeRoleReceiver)

	return m.host.Init()
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
	case pcphost.PeerConnectMsg:
		if msg.Err != nil {
			log.WithError(msg.Err).Debugln("Error connecting to peer:", msg.ID)
			m.host.PeerStates[msg.ID] = pcphost.PeerStateFailedConnecting
		} else {
			log.Debugln("Connected to peer:", msg.ID)
			m.host.PeerStates[msg.ID] = pcphost.PeerStateConnected
			cmds = append(cmds, m.host.StartKeyExchange(m.ctx, msg.ID))
		}
	case pcphost.ShutdownMsg:
		// cleanup
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
