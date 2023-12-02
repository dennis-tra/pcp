package dht

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/tui"
)

const (
	// Timeout for connecting to the bootstrap peers
	bootstrapTimeout = 10 * time.Second

	// Timeout for looking up our data in the DHT
	lookupTimeout = 10 * time.Second

	// Timeout for pushing our data to the DHT.
	provideTimeout = 30 * time.Second
)

var (
	log = logrus.WithField("comp", "dht")

	// The interval between two discover/advertise operations
	tickInterval = 5 * time.Second
)

type Model struct {
	host   host.Host
	dht    *kaddht.IpfsDHT
	sender tea.Sender

	role   discovery.Role
	chanID int

	opCnt int
	ops   map[int]*opRef

	spinner spinner.Model

	BootstrapsPending   int
	BootstrapsSuccesses int
	BootstrapsErrs      []error

	PendingLookups   int
	CompletedLookups int

	PendingProvides    int
	SuccessfulProvides int
	FailedProvides     int
	LatestProvideErr   error

	State State
	Err   error
}

func New(h host.Host, dht *kaddht.IpfsDHT, sender tea.Sender, role discovery.Role, chanID int) *Model {
	return &Model{
		host:           h,
		dht:            dht,
		sender:         sender,
		role:           role,
		chanID:         chanID,
		ops:            map[int]*opRef{},
		spinner:        spinner.New(spinner.WithSpinner(spinner.Dot)),
		BootstrapsErrs: []error{},
		State:          StateIdle,
	}
}

func (m *Model) Init() tea.Cmd {
	log.Traceln("tea init")
	return m.spinner.Tick
}

func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	log.WithField("type", fmt.Sprintf("%T", msg)).Tracef("handle message: %T\n", msg)

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case bootstrapResultMsg:
		m.BootstrapsPending -= 1
		if msg.err != nil {
			m.BootstrapsErrs = append(m.BootstrapsErrs, msg.err)
		} else {
			m.BootstrapsSuccesses += 1
		}

		ref := m.ops[msg.oid]
		delete(m.ops, msg.oid)
		ref.cancel()

		switch m.State {
		case StateStopping:
		case StateBootstrapping:
			if m.BootstrapsSuccesses >= config.Global.ConnThreshold {
				m.State = StateBootstrapped
			} else if m.BootstrapsPending == 0 && m.BootstrapsSuccesses < config.Global.ConnThreshold {
				m.State = StateError
				m.Err = ErrConnThresholdNotReached{BootstrapErrs: m.BootstrapsErrs}
			}
		}

	case advertiseResultMsg:
		ref := m.ops[msg.oid]
		delete(m.ops, msg.oid)
		ref.cancel()

		m.PendingProvides -= 1
		m.SuccessfulProvides += 1
		m.LatestProvideErr = msg.err

		if msg.err != nil {
			m.FailedProvides += 1
		}

		if m.State == StateActive {
			cmds = append(cmds, m.provide(ref.offset))
		}

	case lookupDone:
		ref := m.ops[msg.oid]
		delete(m.ops, msg.oid)
		ref.cancel()

		m.PendingLookups -= 1
		m.CompletedLookups += 1

		if m.State == StateActive {
			cmds = append(cmds, m.lookup(ref.offset))
		}
	}

	if m.State == StateStopping && len(m.ops) == 0 {
		m.State = StateStopped
	}

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if !config.Global.DHT {
		return "-"
	}

	switch m.State {
	case StateIdle:
		style := lipgloss.NewStyle().Faint(true)
		return style.Render("not started")
	case StateBootstrapping:
		return m.spinner.View() + "(bootstrapping)"
	case StateBootstrapped:
		return m.spinner.View() + "(analyzing network)"
	case StateActive:
		switch m.role {
		case discovery.RoleReceiver:
			return m.spinner.View() + "(searching peer)"
		case discovery.RoleSender:
			if m.SuccessfulProvides > 0 {
				return tui.Green.Render("ready")
			}
			if m.PendingProvides > 0 {
				return m.spinner.View() + "(writing to DHT)"
			}
			return m.spinner.View() + tui.Yellow.Render(fmt.Sprintf("(searching peer %d)", m.PendingLookups+m.CompletedLookups))
		default:
			return m.spinner.View() + "(active)"
		}
	case StateStopping:
		return m.spinner.View() + "(stopping)"
	case StateStopped:
		if errors.Is(m.Err, context.Canceled) {
			return tui.Faint.Render("cancelled")
		} else {
			return tui.Faint.Render("stopped")
		}
	case StateError:
		return tui.Red.Render("failed")
	default:
		return tui.Red.Render("unknown state", m.State.String())
	}
}

func (m *Model) logEntry() *logrus.Entry {
	return log.WithFields(logrus.Fields{
		"chanID": m.chanID,
		"state":  m.State.String(),
	})
}

func (m *Model) newOperation(offset time.Duration, timeout time.Duration) (context.Context, int) {
	m.opCnt += 1
	timeoutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	m.ops[m.opCnt] = &opRef{
		id:     m.opCnt,
		offset: offset,
		cancel: cancel,
	}

	return timeoutCtx, m.opCnt
}

type opRef struct {
	id     int
	offset time.Duration
	cancel context.CancelFunc
}
