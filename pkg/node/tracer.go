package node

import (
	"fmt"

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
	n.hpAllowList.Store(p, true)
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

	switch e := evt.Evt.(type) {
	case *holepunch.StartHolePunchEvt:
		n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageStarted}
	case *holepunch.DirectDialEvt:
		if e.Success {
			n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageSucceeded}
		}
	case *holepunch.HolePunchAttemptEvt:
		n.hpStates[evt.Remote].Attempts += 1
	case *holepunch.ProtocolErrorEvt:
		n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageFailed, Err: fmt.Errorf(e.Error)}
	case *holepunch.EndHolePunchEvt:
		if e.Success {
			n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageSucceeded}
		} else {
			n.hpStates[evt.Remote] = &HolePunchState{Stage: HolePunchStageFailed, Err: fmt.Errorf(e.Error)}
		}
	default:
		panic("unexpected hole punch event")
	}
}
