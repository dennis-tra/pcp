package mdns

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dennis-tra/pcp/pkg/discovery"
)

type (
	PeerMsg   peer.AddrInfo
	stopMsg   struct{ reason error }
	updateMsg struct{ offset time.Duration }
)

func (m *Model) Start(offsets ...time.Duration) (*Model, tea.Cmd) {
	if m.State == StateStarted || len(offsets) == 0 {
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

func (m *Model) wait(offset time.Duration) tea.Cmd {
	return func() tea.Msg {
		// restart mDNS service when the new time window arrives.
		deadline := time.Until(discovery.NewID(offset).TimeSlotStart().Add(discovery.TruncateDuration))
		select {
		case <-time.After(deadline):
			return func() tea.Msg {
				return updateMsg{offset: offset}
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
