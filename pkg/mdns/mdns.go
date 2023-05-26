package mdns

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/discovery"
)

var log = logrus.WithField("comp", "mdns")

type State string

const (
	StateIdle    State = "idle"
	StateStarted State = "roaming"
	StateError   State = "error"
	StateStopped State = "stopped"
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "StateIdle"
	case StateStarted:
		return "StateStarted"
	case StateError:
		return "StateError"
	case StateStopped:
		return "StateStopped"
	default:
		return "StateUnknown"
	}
}

// MDNS encapsulates the logic for roaming
// via multicast DNS in the local network.
type MDNS struct {
	host.Host
	ctx      context.Context
	program  *tea.Program
	chanID   int
	services map[time.Duration]mdns.Service
	spinner  spinner.Model
	State    State
	Err      error
}

type (
	PeerMsg   peer.AddrInfo
	stopMsg   struct{ reason error }
	updateMsg struct{ offset time.Duration }
)

func New(ctx context.Context, h host.Host, program *tea.Program, chanID int) *MDNS {
	m := &MDNS{
		Host:    h,
		ctx:     ctx,
		chanID:  chanID,
		program: program,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
	}

	m.reset()

	return m
}

func (m *MDNS) logEntry() *logrus.Entry {
	return log.WithFields(logrus.Fields{
		"chanID": m.chanID,
		"state":  m.State.String(),
	})
}

func (m *MDNS) wait(offset time.Duration) tea.Cmd {
	return func() tea.Msg {
		// restart mDNS service when the new time window arrives.
		deadline := time.Until(discovery.NewID(offset).TimeSlotStart().Add(discovery.TruncateDuration))
		select {
		case <-m.ctx.Done():
			return func() tea.Msg {
				return stopMsg{reason: m.ctx.Err()}
			}
		case <-time.After(deadline):
			return func() tea.Msg {
				return updateMsg{offset: offset}
			}
		}
	}
}

func (m *MDNS) Init() tea.Cmd {
	log.Traceln("tea init")
	return m.spinner.Tick
}

func (m *MDNS) Start(offsets ...time.Duration) (*MDNS, tea.Cmd) {
	if m.State == StateStarted {
		log.Fatal("mDNS service already running")
		return m, nil
	}

	var cmds []tea.Cmd

	m.Err = nil

	for _, offset := range offsets {
		svc, err := m.newService(offset)
		if err != nil {
			m.reset()
			m.State = StateError
			m.Err = fmt.Errorf("start mdns service offset: %w", err)
			return m, nil
		}
		m.services[offset] = svc
	}

	m.State = StateStarted

	for offset := range m.services {
		cmds = append(cmds, m.wait(offset))
	}

	return m, tea.Batch(cmds...)
}

func (m *MDNS) Stop() tea.Cmd {
	return func() tea.Msg {
		return stopMsg{}
	}
}

func (m *MDNS) Update(msg tea.Msg) (*MDNS, tea.Cmd) {
	m.logEntry().WithField("type", fmt.Sprintf("%T", msg)).Tracef("handle message: %T\n", msg)

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case updateMsg:
		if m.State != StateStarted {
			log.Fatal("mDNS service not running")
			return m, nil
		}

		svc, found := m.services[msg.offset]
		if !found {
			return m, nil
		}

		logEntry := m.logEntry().WithField("offset", msg.offset)
		logEntry.Traceln("Updating mDNS service")

		if err := svc.Close(); err != nil {
			log.WithError(err).Warningln("Couldn't close mDNS service")
		}

		svc, err := m.newService(msg.offset)
		if err != nil {
			m.reset()
			m.State = StateError
			m.Err = fmt.Errorf("start mdns service offset: %w", err)
			return m, nil
		}
		m.services[msg.offset] = svc

		cmds = append(cmds, m.wait(msg.offset))

	case stopMsg:
		if m.State != StateStarted {
			return m, nil
		}
		m.logEntry().WithError(msg.reason).Infoln("Stopping mDNS service")

		m.reset()
		m.State = StateStopped
		m.Err = msg.reason
	}

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *MDNS) View() string {
	switch m.State {
	case StateIdle:
		style := lipgloss.NewStyle().Faint(true)
		return style.Render("not started")
	case StateStarted:
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
		return style.Render("ready")
	case StateStopped:
		if errors.Is(m.Err, context.Canceled) {
			style := lipgloss.NewStyle().Faint(true)
			return style.Render("cancelled")
		} else {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
			return style.Render("stopped")
		}
	case StateError:
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		return style.Render("failed")
	default:
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		return style.Render("unknown state")
	}
}

func (m *MDNS) reset() {
	// close already started services
	for _, s := range m.services {
		if err := s.Close(); err != nil {
			log.WithError(err).Warnln("Failed closing mDNS service")
		}
	}

	m.chanID = -1
	m.services = map[time.Duration]mdns.Service{}
	m.State = StateIdle
	m.Err = nil
}

func (m *MDNS) newService(offset time.Duration) (mdns.Service, error) {
	did := discovery.NewID(offset).DiscoveryID(m.chanID)
	logEntry := m.logEntry().
		WithField("did", did).
		WithField("offset", offset.String())
	logEntry.Infoln("Starting mDNS service")

	svc := mdns.NewMdnsService(m, did, m)
	if err := svc.Start(); err != nil {
		logEntry.WithError(err).Warnln("Failed starting mDNS service")
		return nil, fmt.Errorf("start mdns service offset: %w", err)
	}

	return svc, nil
}

func (m *MDNS) HandlePeerFound(pi peer.AddrInfo) {
	logEntry := log.WithFields(logrus.Fields{
		"comp":   "mdns",
		"peerID": pi.ID.String()[:16],
	})

	if pi.ID == m.ID() {
		logEntry.Traceln("Found ourself")
		return
	}

	pi.Addrs = onlyPrivate(pi.Addrs)
	if len(pi.Addrs) == 0 {
		logEntry.Debugln("Peer has no private addresses")
		return
	}

	logEntry.Infoln("Found peer via mDNS!")
	m.program.Send(PeerMsg(pi))
}

// Filter out addresses that are public - only allow private ones.
func onlyPrivate(addrs []ma.Multiaddr) []ma.Multiaddr {
	var routable []ma.Multiaddr
	for _, addr := range addrs {
		if manet.IsPrivateAddr(addr) {
			routable = append(routable, addr)
			log.Debugf("\tprivate - %s\n", addr.String())
		} else {
			log.Debugf("\tpublic - %s\n", addr.String())
		}
	}
	return routable
}
