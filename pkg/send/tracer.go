package send

import (
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
)

func (n *Node) RelayFinderStatus(isActive bool) {
	n.relayFinderActiveLk.Lock()
	n.relayFinderActive = isActive
	n.relayFinderActiveLk.Unlock()
}

func (n *Node) ReservationEnded(cnt int) {
}

func (n *Node) ReservationOpened(cnt int) {
}

func (n *Node) ReservationRequestFinished(isRefresh bool, err error) {
}

func (n *Node) RelayAddressCount(i int) {
}

func (n *Node) RelayAddressUpdated() {
}

func (n *Node) CandidateChecked(supportsCircuitV2 bool) {
}

func (n *Node) CandidateAdded(cnt int) {
}

func (n *Node) CandidateRemoved(cnt int) {
}

func (n *Node) CandidateLoopState(state autorelay.CandidateLoopState) {
}

func (n *Node) ScheduledWorkUpdated(scheduledWork *autorelay.ScheduledWorkTimes) {
}

func (n *Node) DesiredReservations(i int) {
}
