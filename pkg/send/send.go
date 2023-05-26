package send

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/dht"
	pcphost "github.com/dennis-tra/pcp/pkg/host"
	"github.com/dennis-tra/pcp/pkg/mdns"
	"github.com/dennis-tra/pcp/pkg/words"
)

var log = logrus.WithField("comp", "send")

type Model struct {
	ctx               context.Context
	host              *pcphost.Host
	program           *tea.Program
	filepath          string
	relayFinderActive bool
}

func NewState(ctx context.Context, program *tea.Program, filepath string) (*Model, error) {
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
		ctx:      ctx,
		filepath: filepath,
		program:  program,
	}

	opt := libp2p.EnableAutoRelayWithPeerSource(
		model.autoRelayPeerSource,
		autorelay.WithMetricsTracer(model),
		autorelay.WithBootDelay(0),
		autorelay.WithMinCandidates(1),
		autorelay.WithNumRelays(1),
	)

	model.host, err = pcphost.New(ctx, program, wrds, opt)
	if err != nil {
		return nil, err
	}

	return model, nil
}

func (m *Model) Init() tea.Cmd {
	log.Traceln("tea init")
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
	case relayFinderStatus:
		m.relayFinderActive = msg.isActive
	case dht.PeerMsg:
	case mdns.PeerMsg:
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
			m.host.MDNS, cmd = m.host.MDNS.Start(0)
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
		possible, err := m.host.IsDirectConnectivityPossible()
		if err != nil {
			m.host.DHT, cmd = m.host.DHT.StopWithReason(err)
			cmds = append(cmds, cmd)
		} else if possible {
			m.host.DHT, cmd = m.host.DHT.Advertise(0)
			cmds = append(cmds, cmd)
		}
	}

	//	// TODO: if dht + mdns are in error -> stop

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	out := ""

	code := strings.Join(m.host.Words, "-")
	out += fmt.Sprintf("Code is: %s\n", code)
	out += fmt.Sprintf("On the other machine run:\n")
	out += fmt.Sprintf("\tpcp receive %s\n", code)

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
func (m *Model) autoRelayPeerSource(ctx context.Context, num int) <-chan peer.AddrInfo {
	log.Debugln("Looking for auto relay peers...")

	out := make(chan peer.AddrInfo)

	go func() {
		defer log.Debugln("Looking for auto relay peers... Done!")
		defer close(out)

		peerID, err := m.host.IpfsDHT.RoutingTable().GenRandPeerID(0)
		if err != nil {
			log.Debugln("error generating random peer ID:", err.Error())
			return
		}

		closestPeers, err := m.host.IpfsDHT.GetClosestPeers(ctx, peerID.String())
		if err != nil {
			return
		}

		maxLen := len(closestPeers)
		if maxLen > num {
			maxLen = num
		}

		for i := 0; i < maxLen; i++ {
			p := closestPeers[i]

			addrs := m.host.Peerstore().Addrs(p)
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
