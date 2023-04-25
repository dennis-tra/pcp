package node

import (
	"fmt"
	"strconv"

	"github.com/dennis-tra/pcp/internal/log"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
)

type HolePunchState struct {
	Stage    HolePunchStage
	Attempts int
	Err      error
}

type HolePunchStage uint8

const (
	HolePunchStageUnknown = iota
	HolePunchStageStarted
	HolePunchStageSucceeded
	HolePunchStageFailed
)

func (n *Node) AddToHolePunchAllowList(p peer.ID) {
	log.Debugf("Add %s to hole punch trace allow list\n", p.String()[:16])
	n.hpAllowList.Store(p, true)
}

func (n *Node) HolePunchFailed(p peer.ID) <-chan struct{} {
	n.hpStatesLk.Lock()
	defer n.hpStatesLk.Unlock()

	c := make(chan struct{})

	// if the hole punch for the peer is already
	// in the failed stage return early.
	if state, found := n.hpStates[p]; found && state.Stage == HolePunchStageFailed {
		close(c)
		return c
	}

	n.hpFailed[p] = c

	return n.hpFailed[p]
}

func (n *Node) notifyHolePunchFailed(p peer.ID) {
	c, found := n.hpFailed[p]
	if !found {
		return
	}
	close(c)

	delete(n.hpFailed, p)
}

func (n *Node) HolePunchStates() map[peer.ID]HolePunchState {
	n.hpStatesLk.RLock()
	cpy := map[peer.ID]HolePunchState{}
	for p, state := range n.hpStates {
		cpy[p] = HolePunchState{
			Stage:    state.Stage,
			Attempts: state.Attempts,
			Err:      state.Err,
		}
	}
	n.hpStatesLk.RUnlock()

	return cpy
}

func (n *Node) Trace(evt *holepunch.Event) {
	_, ok := n.hpAllowList.Load(evt.Remote)
	if !ok {
		// Check if the remote has initiated the hole punch
		isIncoming := false
		for _, conn := range n.Network().ConnsToPeer(evt.Remote) {
			if conn.Stat().Direction == network.DirInbound {
				isIncoming = true
				break
			}
		}

		if !isIncoming {
			return
		}
	}

	n.hpStatesLk.Lock()
	defer n.hpStatesLk.Unlock()

	if _, found := n.hpStates[evt.Remote]; !found {
		n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageUnknown}
	}

	prefix := fmt.Sprintf("HolePunch %s (%s): ", evt.Remote.String()[:16], evt.Type)
	switch e := evt.Evt.(type) {
	case *holepunch.StartHolePunchEvt:
		log.Debugf(prefix+"rtt=%s\n", e.RTT)
		n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageStarted}
	case *holepunch.DirectDialEvt:
		log.Debugf(prefix+"success=%s err=%v time=%s\n", strconv.FormatBool(e.Success), e.Error, e.EllapsedTime)
		if e.Success {
			n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageSucceeded}
		}
	case *holepunch.HolePunchAttemptEvt:
		log.Debugf(prefix+"attempt=%d\n", e.Attempt)
		n.hpStates[evt.Remote].Attempts += 1
	case *holepunch.ProtocolErrorEvt:
		log.Debugf(prefix+"err=%v\n", e.Error)
		n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageFailed, Err: fmt.Errorf(e.Error)}
		n.notifyHolePunchFailed(evt.Remote)
	case *holepunch.EndHolePunchEvt:
		log.Debugf(prefix+"success=%s err=%v time=%s\n", strconv.FormatBool(e.Success), e.Error, e.EllapsedTime)
		if e.Success {
			n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageSucceeded}
		} else {
			n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageFailed, Err: fmt.Errorf(e.Error)}
			n.notifyHolePunchFailed(evt.Remote)
		}
	default:
		panic("unexpected hole punch event")
	}
}
