package receive

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/discovery"
	pcphost "github.com/dennis-tra/pcp/pkg/host"
)

var log = logrus.WithField("comp", "receive")

type Model struct {
	ctx     context.Context
	host    *pcphost.Model
	program *tea.Program
}

func NewState(ctx context.Context, program *tea.Program, words []string) (*Model, error) {
	if !config.Global.DHT && !config.Global.MDNS {
		return nil, fmt.Errorf("either the DHT or mDNS discovery mechanism need to be active")
	}

	log.Infoln("Random words:", strings.Join(words, "-"))

	// Start the libp2p node
	h, err := pcphost.New(ctx, program, discovery.RoleReceiver, words)
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

	m.host.RegisterKeyExchangeHandler()

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
		if msg.ID.String() == "" {
			panic("jjjjjjjjjjjjjj")
		}
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
		cmds = append(cmds, tea.Quit)
	}

	switch m.host.DHT.State {
	case dht.StateIdle:
		if config.Global.DHT {
			m.host.DHT, cmd = m.host.DHT.Bootstrap()
			cmds = append(cmds, cmd)
		}
	case dht.StateBootstrapping:
	case dht.StateBootstrapped:
		m.host.DHT, cmd = m.host.DHT.StartNoProvide()
		cmds = append(cmds, cmd)
	case dht.StateActive:
		possible, _ := m.host.IsDirectConnectivityPossible()
		if possible && m.host.DHT.PendingProvides == 0 {
			m.host.DHT, cmd = m.host.DHT.StartProvide()
			cmds = append(cmds, cmd)
		}
	}

	//	// TODO: if dht + mdns are in error -> stop

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	out := ""

	code := strings.Join(m.host.Words, "-")
	out += fmt.Sprintf("Looking for peer %s...\n", code)

	out += m.host.View()

	return out
}
