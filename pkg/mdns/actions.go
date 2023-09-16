package mdns

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/dennis-tra/pcp/pkg/discovery"
)

type (
	PeerMsg   peer.AddrInfo
	stopMsg   struct{ reason error }
	updateMsg struct{ serviceID int }
)

// Start starts as many MDNS services as offsets are given.
func (m *Model) Start(offsets ...time.Duration) (*Model, tea.Cmd) {
	if m.State == StateStarted || len(offsets) == 0 {
		log.Fatal("mDNS service already running")
		return m, nil
	}

	m.reset()

	var cmds []tea.Cmd
	for _, offset := range offsets {
		svc, err := m.newService(offset)
		if err != nil {
			m.reset()
			m.State = StateError
			m.Err = fmt.Errorf("start mdns service offset: %w", err)
			return m, nil
		}

		m.serviceID += 1
		m.services[m.serviceID] = &serviceRef{
			id:     m.serviceID,
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

func (m *Model) StopWithReason(reason error) (*Model, tea.Cmd) {
	m.reset()
	if reason != nil && !errors.Is(reason, context.Canceled) {
		m.State = StateError
		m.Err = reason
	} else {
		m.State = StateStopped
	}
	return m, nil
}
