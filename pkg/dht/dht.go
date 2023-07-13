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
)

const (
	// Timeout for looking up our data in the DHT
	lookupTimeout = 10 * time.Second
)

var (
	log = logrus.WithField("comp", "dht")

	// Timeout for pushing our data to the DHT.
	provideTimeout = 30 * time.Second

	// The interval between two discover/advertise operations
	tickInterval = 5 * time.Second
)

type DHT struct {
	host.Host
	ctx context.Context
	dht *kaddht.IpfsDHT

	chanID   int
	services map[time.Duration]context.CancelFunc
	spinner  spinner.Model

	BootstrapsPending   int
	BootstrapsSuccesses int
	BootstrapsErrs      []error

	State State
	Err   error
}

func New(ctx context.Context, h host.Host, dht *kaddht.IpfsDHT, chanID int) *DHT {
	return &DHT{
		ctx:      ctx,
		Host:     h,
		dht:      dht,
		chanID:   chanID,
		services: map[time.Duration]context.CancelFunc{},
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
		State:    StateIdle,
	}
}

func (d *DHT) Init() tea.Cmd {
	log.Traceln("tea init")
	return d.spinner.Tick
}

func (d *DHT) Update(msg tea.Msg) (*DHT, tea.Cmd) {
	log.WithField("type", fmt.Sprintf("%T", msg)).Tracef("handle message: %T\n", msg)

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case advertiseResultMsg:
		if d.State == StateStopped || d.State == StateError {
			return d, nil
		}
		cancel, found := d.services[msg.offset]
		if !found {
			log.Fatal("DHT service not found")
			return d, nil
		}
		cancel()

		if msg.err == nil {
			d.State = StateProvided
		} else if msg.err != nil && msg.fatal {
			d.State = StateError
			d.Err = msg.err
			return d, nil
		} else if errors.Is(msg.err, context.Canceled) && d.State != StateStopped {
			d.State = StateStopped
			d.Err = msg.err
			return d, nil
		} else {
			d.State = StateRetrying
		}

		provideCtx, cancel := context.WithTimeout(d.ctx, provideTimeout)
		d.services[msg.offset] = cancel
		cmds = append(cmds, d.provide(provideCtx, msg.offset))
	case bootstrapResultMsg:
		d.BootstrapsPending -= 1
		if msg.err != nil {
			d.BootstrapsErrs = append(d.BootstrapsErrs, msg.err)
		} else {
			d.BootstrapsSuccesses += 1
		}

		if d.State == StateBootstrapping {
			if d.BootstrapsSuccesses >= ConnThreshold {
				d.State = StateBootstrapped
			} else if d.BootstrapsPending == 0 && d.BootstrapsSuccesses < ConnThreshold {
				d.reset()
				d.State = StateError
				d.Err = ErrConnThresholdNotReached{BootstrapErrs: d.BootstrapsErrs}
			}
		}

	case PeerMsg:
		if d.State == StateStopped || d.State == StateError {
			return d, nil
		}

		cancel, found := d.services[msg.offset]
		if !found {
			log.Fatal("DHT service not found")
			return d, nil
		}
		cancel()

		if msg.Err == nil {
			d.State = StateLookup
		} else if msg.Err != nil && msg.fatal {
			d.State = StateError
			d.Err = msg.Err
			return d, nil
		} else if errors.Is(msg.Err, context.Canceled) && d.State != StateStopped {
			d.State = StateStopped
			d.Err = msg.Err
			return d, nil
		} else {
			d.State = StateRetrying
		}

		lookupCtx, cancel := context.WithCancel(d.ctx)
		d.services[msg.offset] = cancel
		cmds = append(cmds, d.lookup(lookupCtx, msg.offset))

	case stopMsg:
		d, cmd = d.StopWithReason(msg.reason)
		cmds = append(cmds, cmd)

	}

	d.spinner, cmd = d.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return d, tea.Batch(cmds...)
}

func (d *DHT) View() string {
	if !config.Global.DHT {
		return "-"
	}

	switch d.State {
	case StateIdle:
		style := lipgloss.NewStyle().Faint(true)
		return style.Render("not started")
	case StateBootstrapping:
		return d.spinner.View() + "(bootstrapping)"
	case StateBootstrapped:
		return d.spinner.View() + "(analyzing network)"
	case StateProviding:
		return d.spinner.View() + "(writing to DHT)"
	case StateLookup:
		return d.spinner.View() + "(searching peer)"
	case StateRetrying:
		return d.spinner.View() + "(retrying)"
	case StateProvided:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("ready")
	case StateStopped:
		style := lipgloss.NewStyle().Faint(true)
		if errors.Is(d.Err, context.Canceled) {
			return style.Render("cancelled")
		} else {
			return style.Render("stopped")
		}
	case StateError:
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		return style.Render("failed")
	default:
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		return style.Render("unknown state", d.State.String())
	}
}

func (d *DHT) logEntry() *logrus.Entry {
	return log.WithFields(logrus.Fields{
		"chanID": d.chanID,
		"state":  d.State.String(),
	})
}

func (d *DHT) reset() {
	// close already started services
	for _, cancel := range d.services {
		cancel()
	}

	d.services = map[time.Duration]context.CancelFunc{}
	d.State = StateIdle
	d.Err = nil
}
