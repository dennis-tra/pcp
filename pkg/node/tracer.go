package node

import (
	"fmt"
	"strconv"

	log "github.com/sirupsen/logrus"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
)

// HolePunchState encapsulates all information about the current state
// that we are in during the hole punch exchange. The notifyFailed
// channel is closed when the hole punch failed and listeners
// were registered.
type HolePunchState struct {
	Stage    HolePunchStage
	Attempts int
	Err      error

	notifyFailed chan struct{}
}

type HolePunchStage uint8

const (
	HolePunchStageUnknown = iota
	HolePunchStageStarted
	HolePunchStageSucceeded
	HolePunchStageFailed
)

// AddToHolePunchAllowList puts the given peer into a list that tells the
// hole punch tracer to keep track of its hole punch state.
func (n *Node) AddToHolePunchAllowList(p peer.ID) {
	log.Debugf("Add %s to hole punch trace allow list\n", p.String()[:16])
	n.hpAllowList.Store(p, true)
}

// RegisterHolePunchFailed returns a channel that is closed when all hole punch attempts have failed
func (n *Node) RegisterHolePunchFailed(p peer.ID) <-chan struct{} {
	log.Debugln("Register for hole puch failed events ", p.String())
	n.hpStatesLk.Lock()
	defer n.hpStatesLk.Unlock()

	// if the hole punch for the peer is already
	// in the failed stage return early.
	state, found := n.hpStates[p]
	if !found {
		state = &HolePunchState{
			Stage:        HolePunchStageUnknown,
			Attempts:     0,
			Err:          nil,
			notifyFailed: make(chan struct{}),
		}
		n.hpStates[p] = state
	}

	if state.Stage == HolePunchStageFailed && state.Attempts >= 3 {
		close(state.notifyFailed)
		return state.notifyFailed
	}

	return n.hpStates[p].notifyFailed
}

func (n *Node) UnregisterHolePunchFailed(p peer.ID) {
	log.Debugln("Unregister for hole punch failed events ", p.String())
	n.hpStatesLk.Lock()
	defer n.hpStatesLk.Unlock()

	state, found := n.hpStates[p]
	if !found {
		return
	}

	state.notifyFailed = make(chan struct{})
}

// notifyHolePunchFailed must be called when the lock of hpFailed is hold.
func (n *Node) notifyHolePunchFailed(p peer.ID) {
	state, found := n.hpStates[p]
	if !found {
		return
	}
	state.Attempts += 1

	n.hpStates[p] = state // necessary?

	// only notifyFailed listeners after three hole punch attempts have failed
	if state.Attempts < 3 {
		return
	}

	// notifyFailed listeners
	close(state.notifyFailed)

	delete(n.hpStates, p)
}

// HolePunchStates returns a copy of the internal HolePunchStates.
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

// Trace is the holepunch.Tracer interface implementation
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
		n.hpStates[evt.Remote] = &HolePunchState{
			Stage:        HolePunchStageUnknown,
			notifyFailed: make(chan struct{}),
		}
	}

	prefix := fmt.Sprintf("HolePunch %s (%s): ", evt.Remote.String()[:16], evt.Type)
	switch e := evt.Evt.(type) {
	case *holepunch.StartHolePunchEvt:
		log.Debugf(prefix+"rtt=%s\n", e.RTT)
		n.hpStates[evt.Remote].Stage = HolePunchStageStarted
	case *holepunch.DirectDialEvt:
		log.Debugf(prefix+"success=%s err=%v time=%s\n", strconv.FormatBool(e.Success), e.Error, e.EllapsedTime)
		if e.Success {
			n.hpStates[evt.Remote].Stage = HolePunchStageSucceeded
		}
	case *holepunch.HolePunchAttemptEvt:
		log.Debugf(prefix+"attempt=%d\n", e.Attempt)
		n.hpStates[evt.Remote].Attempts += 1
	case *holepunch.ProtocolErrorEvt:
		log.Debugf(prefix+"err=%v\n", e.Error)
		n.hpStates[evt.Remote].Stage = HolePunchStageFailed
		n.hpStates[evt.Remote].Err = fmt.Errorf(e.Error)
		n.notifyHolePunchFailed(evt.Remote)
	case *holepunch.EndHolePunchEvt:
		log.Debugf(prefix+"success=%s err=%v time=%s\n", strconv.FormatBool(e.Success), e.Error, e.EllapsedTime)
		if e.Success {
			n.hpStates[evt.Remote].Stage = HolePunchStageSucceeded
			n.hpStates[evt.Remote].Err = nil
		} else {
			n.hpStates[evt.Remote].Stage = HolePunchStageFailed
			n.hpStates[evt.Remote].Err = fmt.Errorf(e.Error)
			n.notifyHolePunchFailed(evt.Remote)
		}
	default:
		panic("unexpected hole punch event")
	}
}
