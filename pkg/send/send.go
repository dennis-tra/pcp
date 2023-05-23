package send

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/events"
	pcphost "github.com/dennis-tra/pcp/pkg/host"
	"github.com/dennis-tra/pcp/pkg/mdns"
	"github.com/dennis-tra/pcp/pkg/words"
)

var log = logrus.WithField("comp", "send")

type Model struct {
	ctx               context.Context
	host              *pcphost.Host
	filepath          string
	rfsEmitter        *events.Emitter[relayFinderStatus]
	relayFinderActive bool
}

func NewState(ctx context.Context, filepath string) (*Model, error) {
	if !config.Global.DHT && !config.Global.MDNS {
		return nil, fmt.Errorf("either the DHT or mDNS discovery mechanism need to be active")
	}

	// Try to open the file to check if we have access and fail early.
	if err := validateFile(filepath); err != nil {
		return nil, err
	}

	// Generate the random words
	_, wrds, err := words.Random("english", config.Send.WordCount)
	if err != nil {
		return nil, err
	}

	// If homebrew flag is set, overwrite generated words with well known list
	if config.Global.Homebrew {
		wrds = words.HomebrewList()
	}

	log.Infoln("Random words:", strings.Join(wrds, "-"))

	// Start the libp2p node
	model := &Model{
		ctx:        ctx,
		filepath:   filepath,
		rfsEmitter: events.NewEmitter[relayFinderStatus](),
	}

	opt := libp2p.EnableAutoRelayWithPeerSource(
		model.autoRelayPeerSource,
		autorelay.WithMetricsTracer(model),
		autorelay.WithBootDelay(0),
		autorelay.WithMinCandidates(1),
		autorelay.WithNumRelays(1),
	)

	model.host, err = pcphost.New(ctx, wrds, opt)
	if err != nil {
		return nil, err
	}

	return model, nil
}

func (s *Model) Init() tea.Cmd {
	log.Traceln("tea init")

	return tea.Batch(
		s.rfsEmitter.Subscribe,
		s.host.Init(),
	)
}

func (s *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	log.WithField("type", fmt.Sprintf("%T", msg)).Tracef("handle message: %T\n", msg)

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	s.host, cmd = s.host.Update(msg)
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case events.Msg[relayFinderStatus]:
		s.relayFinderActive = msg.Value.isActive
	case events.Msg[dht.PeerMsg]:
		cmds = append(cmds, msg.Listen)
	case events.Msg[mdns.PeerMsg]:
		cmds = append(cmds, msg.Listen)
	case syscall.Signal:
		cmds = append(cmds, s.handleSignal(msg))
	case tea.KeyMsg:
		cmds = append(cmds, s.handleKeyMsg(msg))
	case pcphost.ShutdownMsg:
		return s, tea.Quit
	}

	switch s.host.MDNS.State {
	case mdns.StateIdle:
		if len(s.host.PrivateAddrs) > 0 {
			s.host.MDNS, cmd = s.host.MDNS.Start(s.host.ChanID, time.Duration(0))
			cmds = append(cmds, cmd)
		}
	}

	switch s.host.DHT.State {
	case dht.StateIdle:
		s.host.DHT, cmd = s.host.DHT.Bootstrap()
		cmds = append(cmds, cmd)
	case dht.StateBootstrapping:
	case dht.StateBootstrapped:
		possible, err := s.host.IsDirectConnectivityPossible()
		if err != nil {
			s.host.DHT, cmd = s.host.DHT.StopWithReason(err)
			cmds = append(cmds, cmd)
		} else if possible {
			s.host.DHT, cmd = s.host.DHT.Advertise(s.host.ChanID, 0)
			cmds = append(cmds, cmd)
		}
	}

	//	// TODO: if dht + mdns are in error -> stop

	return s, tea.Batch(cmds...)
}

func (s *Model) View() string {
	out := ""

	code := strings.Join(s.host.Words, "-")
	out += fmt.Sprintf("Code is: %s\n", code)
	out += fmt.Sprintf("On the other machine run:\n")
	out += fmt.Sprintf("\tpcp receive %s\n", code)

	style := lipgloss.NewStyle().Bold(true)

	out += s.host.View()

	internet := s.host.DHT.View()
	if s.host.Reachability == network.ReachabilityPrivate && s.relayFinderActive && s.host.DHT.State == dht.StateBootstrapped {
		internet = "finding signaling peers"
	}

	out += fmt.Sprintf("%s %s\t %s %s\n", style.Render("Local Network:"), s.host.MDNS.View(), style.Render("Internet:"), internet)

	return out
}

func (s *Model) handleSignal(sig syscall.Signal) tea.Cmd {
	log.WithField("sig", sig.String()).Infoln("received signal")
	return tea.Quit
}

func (s *Model) handleKeyMsg(keyMsg tea.KeyMsg) tea.Cmd {
	switch keyMsg.Type {
	}
	return nil
}

func validateFile(filepath string) error {
	log.Debugln("Validating given file:", filepath)

	if filepath == "" {
		return fmt.Errorf("please specify the file you want to transfer")
	}

	// checks if file exists and we have read permissions
	_, err := os.Stat(filepath)
	return err
}

// autoRelayPeerSource is a function that queries the DHT for a random peer ID with CPL 0.
// The found peers are used as candidates for circuit relay v2 peers.
func (s *Model) autoRelayPeerSource(ctx context.Context, num int) <-chan peer.AddrInfo {
	log.Debugln("Looking for auto relay peers...")

	out := make(chan peer.AddrInfo)

	go func() {
		defer log.Debugln("Looking for auto relay peers... Done!")
		defer close(out)

		peerID, err := s.host.IpfsDHT.RoutingTable().GenRandPeerID(0)
		if err != nil {
			log.Debugln("error generating random peer ID:", err.Error())
			return
		}

		closestPeers, err := s.host.IpfsDHT.GetClosestPeers(ctx, peerID.String())
		if err != nil {
			return
		}

		maxLen := len(closestPeers)
		if maxLen > num {
			maxLen = num
		}

		for i := 0; i < maxLen; i++ {
			p := closestPeers[i]

			addrs := s.host.Peerstore().Addrs(p)
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
