package dht

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/events"
)

var log = logrus.WithField("comp", "dht")

// Timeout for pushing our data to the DHT.
var (
	provideTimeout = 30 * time.Second
)

// The interval between two discover/advertise operations
var (
	tickInterval = 5 * time.Second
)

const (
	// Timeout for looking up our data in the DHT
	lookupTimeout = 10 * time.Second
)

type DHT struct {
	host.Host
	ctx context.Context
	dht *kaddht.IpfsDHT

	emitter  *events.Emitter[PeerMsg]
	chanID   int
	services map[time.Duration]context.CancelFunc
	spinner  spinner.Model

	// flags
	IsBootstrapped bool

	State State
	Err   error
}

type (
	PeerMsg struct {
		Peer   peer.AddrInfo
		offset time.Duration
		err    error
		fatal  bool
	}
	advertiseMsg struct {
		chanID  int
		offsets []time.Duration
	}
	advertiseResult struct {
		offset time.Duration
		err    error
		fatal  bool
	}
	discoverMsg struct {
		chanID  int
		offsets []time.Duration
	}
	stopMsg struct{ reason error }
)

func (d *DHT) Bootstrap() (*DHT, tea.Cmd) {
	if !(d.IsBootstrapped || d.State == StateBootstrapping) {
		d.State = StateBootstrapping
		return d, d.connectToBootstrapper
	} else {
		return d, nil
	}
}

func (d *DHT) Advertise(chanID int, offsets ...time.Duration) tea.Cmd {
	return func() tea.Msg {
		return advertiseMsg{chanID: chanID, offsets: offsets}
	}
}

func (d *DHT) Discover(chanID int, offsets ...time.Duration) tea.Cmd {
	return func() tea.Msg {
		return discoverMsg{chanID: chanID, offsets: offsets}
	}
}

func (d *DHT) Stop() tea.Cmd {
	return func() tea.Msg {
		return stopMsg{}
	}
}

func (d *DHT) StopWithReason(reason error) tea.Cmd {
	return func() tea.Msg {
		return stopMsg{reason: reason}
	}
}

func New(ctx context.Context, h host.Host, dht *kaddht.IpfsDHT) *DHT {
	return &DHT{
		ctx:      ctx,
		Host:     h,
		dht:      dht,
		services: map[time.Duration]context.CancelFunc{},
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
		State:    StateIdle,
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

	if d.emitter != nil {
		d.emitter.Close()
	}

	d.chanID = -1
	d.emitter = nil
	d.services = map[time.Duration]context.CancelFunc{}
	d.State = StateIdle
	d.Err = nil
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
	case advertiseMsg:

		if d.State == StateProviding || d.State == StateLookup {
			log.Fatal("DHT service already running")
			return d, nil
		}

		d.chanID = msg.chanID
		d.Err = nil

		for _, offset := range msg.offsets {
			if _, found := d.services[offset]; found {
				continue
			}

			provideCtx, cancel := context.WithTimeout(d.ctx, provideTimeout)
			d.services[offset] = cancel
			cmds = append(cmds, d.provide(provideCtx, offset))
		}

		d.State = StateProviding

	case advertiseResult:
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
	case bootstrapResult:
		if msg.err != nil {
			d.reset()
			d.State = StateError
			d.Err = msg.err
		} else if d.State == StateBootstrapping {
			d.IsBootstrapped = true
			d.State = StateBootstrapped
		}

	case discoverMsg:
		if d.State == StateProviding || d.State == StateLookup {
			log.Fatal("DHT service already running")
			return d, nil
		}

		d.chanID = msg.chanID
		d.emitter = events.NewEmitter[PeerMsg]()
		d.Err = nil

		for _, offset := range msg.offsets {
			if _, found := d.services[offset]; found {
				continue
			}

			lookupCtx, cancel := context.WithCancel(d.ctx)
			d.services[offset] = cancel
			cmds = append(cmds, d.lookup(lookupCtx, offset))
		}

		d.State = StateLookup

	case PeerMsg:
		cancel, found := d.services[msg.offset]
		if !found {
			log.Fatal("DHT service not found")
			return d, nil
		}
		cancel()

		if msg.err == nil {
			d.State = StateLookup
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

		lookupCtx, cancel := context.WithCancel(d.ctx)
		d.services[msg.offset] = cancel
		cmds = append(cmds, d.lookup(lookupCtx, msg.offset))

	case stopMsg:
		d.reset()
		if msg.reason != nil {
			d.State = StateError
			d.Err = msg.reason
		} else {
			d.State = StateStopped
		}
	}

	d.spinner, cmd = d.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return d, tea.Batch(cmds...)
}

func (d *DHT) View() string {
	switch d.State {
	case StateIdle:
		style := lipgloss.NewStyle().Faint(true)
		return style.Render("not started")
	case StateBootstrapping:
		return d.spinner.View() + "(bootstrapping)"
	case StateBootstrapped:
		// style := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
		// return style.Render("bootstrapped")
		return d.spinner.View() + "(analyzing network)"
	case StateProviding:
		return d.spinner.View() + "(writing to DHT)"
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
		return style.Render("unknown state")
	}
}

func (d *DHT) provide(ctx context.Context, offset time.Duration) tea.Cmd {
	return func() tea.Msg {
		did := discovery.NewID(offset)

		logEntry := log.WithField("did", did.DiscoveryID(d.chanID))
		logEntry.Debugln("Start providing")
		defer logEntry.Debugln("Done providing")

		cID, err := did.ContentID(did.DiscoveryID(d.chanID))
		if err != nil {
			return advertiseResult{
				offset: offset,
				err:    err,
				fatal:  true,
			}
		}

		return advertiseResult{
			offset: offset,
			err:    d.dht.Provide(ctx, cID, true),
		}
	}
}

func (d *DHT) lookup(ctx context.Context, offset time.Duration) tea.Cmd {
	return func() tea.Msg {
		did := discovery.NewID(offset)

		logEntry := log.WithField("did", did.DiscoveryID(d.chanID))
		logEntry.Debugln("Start lookup")
		defer logEntry.Debugln("Done lookup")

		cID, err := did.ContentID(did.DiscoveryID(d.chanID))
		if err != nil {
			return PeerMsg{
				offset: offset,
				err:    err,
				fatal:  true,
			}
		}

		// Find new provider with a timeout, so the discovery ID is renewed if necessary.
		ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
		defer cancel()
		for pi := range d.dht.FindProvidersAsync(ctx, cID, 0) {
			log.Debugln("DHT - Found peer ", pi.ID)
			if len(pi.Addrs) > 0 {
				return PeerMsg{
					Peer:   pi,
					offset: offset,
				}
			}
		}

		return PeerMsg{
			err: fmt.Errorf("not found"),
		}
	}
}

type bootstrapResult struct {
	err error
}

// connectToBootstrapper connects to a set of bootstrap nodes to connect
// to the DHT.
func (d *DHT) connectToBootstrapper() tea.Msg {
	log.Infoln("Start bootstrapping")

	var peers []peer.AddrInfo
	for _, maddrStr := range config.Global.BootstrapPeers.Value() {

		maddr, err := ma.NewMultiaddr(maddrStr)
		if err != nil {
			log.WithError(err).WithField("maddr", maddrStr).Warnln("Couldn't parse multiaddress")
			continue
		}

		pi, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.WithError(err).WithField("maddr", maddr).Warningln("Couldn't craft peer addr info")
			continue
		}

		peers = append(peers, *pi)
	}

	peerCount := len(peers)
	if peerCount < 4 {
		return bootstrapResult{err: fmt.Errorf("too few Bootstrap peers configured (min %d)", ConnThreshold)}
	}

	// Asynchronously connect to all bootstrap peers and send
	// potential errors to a channel. This channel is used
	// to capture the errors and check if we have established
	// enough connections. An error group (errgroup) cannot
	// be used here as it exits as soon as an error is thrown
	// in one of the Go-Routines.
	var wg sync.WaitGroup
	errChan := make(chan error, peerCount)

	for _, bp := range peers {
		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			logEntry := log.WithField("peerID", pi.ID.String()[:16])
			logEntry.Debugln("Connecting bootstrap peer")
			err := d.Connect(d.ctx, pi)
			if err != nil {
				logEntry.WithError(err).Warnln("Failed connecting to bootstrap peer")
			} else {
				logEntry.Infoln("Connected to bootstrap peer!")
			}
			errChan <- err
		}(bp)
	}

	// Close error channel after all connection attempts are done
	// to signal the for-loop below to stop.
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Reading the error channel and collect errors.
	errs := ErrConnThresholdNotReached{BootstrapErrs: []error{}}
	for {
		err, ok := <-errChan
		if !ok {
			// channel was closed.
			break
		} else if err != nil {
			errs.BootstrapErrs = append(errs.BootstrapErrs, err)
		}
	}

	// If we could not establish enough connections return an error
	if peerCount-len(errs.BootstrapErrs) < ConnThreshold {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		default:
			return bootstrapResult{err: errs}
		}
	}

	return bootstrapResult{err: nil}
}
