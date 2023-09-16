package send

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/discovery"
	pcphost "github.com/dennis-tra/pcp/pkg/host"
	"github.com/dennis-tra/pcp/pkg/mdns"
	"github.com/dennis-tra/pcp/pkg/words"
)

var log = logrus.WithField("comp", "send")

type Model struct {
	ctx      context.Context
	host     *pcphost.Model
	program  *tea.Program
	filepath string
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

	// If homebrew flag is set, overwrite generated words with well-known list
	if config.Global.Homebrew {
		wrds = words.HomebrewList()
	}

	log.Infoln("Random words:", strings.Join(wrds, "-"))

	model := &Model{
		ctx:      ctx,
		filepath: filepath,
		program:  program,
	}

	model.host, err = pcphost.New(ctx, program, discovery.RoleSender, wrds)
	if err != nil {
		return nil, err
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

	switch msg.(type) {
	case pcphost.ShutdownMsg:
		// cleanup
		cmds = append(cmds, tea.Quit)
	}

	switch m.host.MDNS.State {
	case mdns.StateIdle:
		if config.Global.MDNS && len(m.host.PrivateAddrs) > 0 {
			m.host.MDNS, cmd = m.host.MDNS.Start(0, 10*time.Second)
			cmds = append(cmds, cmd)
		}
	}

	switch m.host.DHT.State {
	case dht.StateIdle:
		if config.Global.DHT {
			m.host.DHT, cmd = m.host.DHT.Bootstrap()
			cmds = append(cmds, cmd)
		}
	case dht.StateBootstrapped:
		possible, err := m.host.IsDirectConnectivityPossible()
		if err != nil {
			m.host.DHT, cmd = m.host.DHT.StopWithReason(err)
		} else if possible {
			m.host.DHT, cmd = m.host.DHT.Advertise(0)
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	out := ""

	code := strings.Join(m.host.Words, "-")
	out += fmt.Sprintf("Code is: %s\n", code)
	out += fmt.Sprintf("On the other machine run:\n")
	out += fmt.Sprintf("\tpcp receive %s\n", code)

	out += m.host.View()

	return out
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
