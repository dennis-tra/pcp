package mdns

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/dennis-tra/pcp/pkg/discovery"
)

type (
	PeerMsg   peer.AddrInfo
	updateMsg struct{ serviceID int }
)

// Start starts as many MDNS services as offsets are given.
func (m *Model) Start() (*Model, tea.Cmd) {
	m.reset()

	var cmds []tea.Cmd
	for _, offset := range []time.Duration{0, discovery.TruncateDuration} {
		svc, err := m.newService(offset)
		if err != nil {
			m.reset()
			m.State = StateError
			m.Err = fmt.Errorf("start mdns service offset: %w", err)
			return m, nil
		}

		m.svcIdCntr += 1
		m.services[m.svcIdCntr] = &serviceRef{
			id:     m.svcIdCntr,
			offset: offset,
			svc:    svc,
		}

	}

	m.State = StateStarted

	for _, ref := range m.services {
		cmds = append(cmds, m.wait(ref))
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) wait(ref *serviceRef) tea.Cmd {
	return func() tea.Msg {
		// restart mDNS service when the new time window arrives.
		deadline := discovery.NewID(m.chanID).
			SetOffset(ref.offset).
			TimeSlotStart().
			Add(discovery.TruncateDuration)

		select {
		case <-time.After(time.Until(deadline)):
			return func() tea.Msg {
				return updateMsg{serviceID: ref.id}
			}
		}
	}
}

func (m *Model) Stop() (*Model, tea.Cmd) {
	m.reset()
	m.State = StateStopped
	return m, nil
}
