package send

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
)

type relayFinderStatus struct {
	isActive bool
}

func (s *Model) RelayFinderStatus(isActive bool) {
	s.rfsEmitter.Emit(relayFinderStatus{isActive: isActive})
}

func (s *Model) ReservationEnded(cnt int) {}

func (s *Model) ReservationOpened(cnt int) {}

func (s *Model) ReservationRequestFinished(isRefresh bool, err error) {}

func (s *Model) RelayAddressCount(i int) {}

func (s *Model) RelayAddressUpdated() {}

func (s *Model) CandidateChecked(supportsCircuitV2 bool) {}

func (s *Model) CandidateAdded(cnt int) {}

func (s *Model) CandidateRemoved(cnt int) {}

func (s *Model) CandidateLoopState(state autorelay.CandidateLoopState) {}

func (s *Model) ScheduledWorkUpdated(scheduledWork *autorelay.ScheduledWorkTimes) {}

func (s *Model) DesiredReservations(i int) {}

func (s *Model) HandleSuccessfulKeyExchange(peerID peer.ID) {
}
