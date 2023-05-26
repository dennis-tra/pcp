package send

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
)

type relayFinderStatus struct {
	isActive bool
}

func (m *Model) RelayFinderStatus(isActive bool) {
	m.program.Send(relayFinderStatus{isActive: isActive})
}

func (m *Model) ReservationEnded(cnt int) {}

func (m *Model) ReservationOpened(cnt int) {}

func (m *Model) ReservationRequestFinished(isRefresh bool, err error) {}

func (m *Model) RelayAddressCount(i int) {}

func (m *Model) RelayAddressUpdated() {}

func (m *Model) CandidateChecked(supportsCircuitV2 bool) {}

func (m *Model) CandidateAdded(cnt int) {}

func (m *Model) CandidateRemoved(cnt int) {}

func (m *Model) CandidateLoopState(state autorelay.CandidateLoopState) {}

func (m *Model) ScheduledWorkUpdated(scheduledWork *autorelay.ScheduledWorkTimes) {}

func (m *Model) DesiredReservations(i int) {}

func (m *Model) HandleSuccessfulKeyExchange(peerID peer.ID) {
}
