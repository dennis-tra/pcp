package receive

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dennis-tra/pcp/pkg/node"

	"github.com/dennis-tra/pcp/pkg/dht"

	"github.com/dennis-tra/pcp/pkg/mdns"

	"github.com/dennis-tra/pcp/internal/log"
)

var spinnerChars = []string{"⠋ ", "⠙ ", "⠹ ", "⠸ ", "⠼ ", "⠴ ", "⠦ ", "⠧ ", "⠇ ", "⠏ "}

func (n *Node) printStatus(stop chan struct{}) {
	n.printStatusWg.Add(1)
	defer n.printStatusWg.Done()

	log.Infoln("On the other machine run:")
	log.Infoln("\tpeercp receive", strings.Join(n.Words, "-"))

	eraseFn := log.DiscoverStatus(n.discoverStatus(spinnerChars[0]), n.verbose)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for i := 0; ; i++ {
		spinnerChar := spinnerChars[i%len(spinnerChars)]
		select {
		case <-stop:
			eraseFn()
			eraseFn = log.DiscoverStatus(n.discoverStatus(spinnerChar), n.verbose)
			return
		case <-n.ServiceContext().Done():
			return
		case <-ticker.C:
			eraseFn()
			eraseFn = log.DiscoverStatus(n.discoverStatus(spinnerChar), n.verbose)
		}
	}
}

func (n *Node) discoverStatus(spinnerChar string) log.DiscoverStatusParams {
	isCtxCancelled := n.ServiceContext().Err() == context.Canceled

	mdnsState := n.mdnsDiscoverer.State()
	dhtState := n.dhtDiscoverer.State()
	pakeStates := n.PakeProtocol.PakeStates()
	hpStates := n.HolePunchStates()

	asp := log.DiscoverStatusParams{
		Code:       strings.Join(n.Words, "-"),
		LANState:   log.Gray("-"),
		MDNSState:  log.Gray("-"),
		Peers:      []string{},
		PeerStates: map[string]string{},
	}

	switch mdnsState.Stage {
	case mdns.StageIdle:
		asp.MDNSState = log.Gray("-")
		asp.LANState = log.Green("-")
	case mdns.StageRoaming:
		asp.MDNSState = spinnerChar
		asp.LANState = log.Green("searching " + spinnerChar)
	case mdns.StageError:
		asp.MDNSState = log.Red(mdnsState.Err.Error())
		asp.LANState = log.Red("failed")
	case mdns.StageStopped:
		asp.MDNSState = log.Green("inactive")
		asp.LANState = log.Green("stopped")
	}

	switch dhtState.Stage {
	case dht.StageIdle:
		asp.DHTState = log.Gray("-")
		asp.WANState = log.Green("-")
	case dht.StageBootstrapping:
		if isCtxCancelled {
			asp.DHTState = log.Gray("cancelled")
			asp.WANState = log.Gray("cancelled")
		} else {
			asp.DHTState = spinnerChar + "(bootstrapping)"
			asp.WANState = spinnerChar + "(bootstrapping)"
		}
	case dht.StageLookup:
		fallthrough
	case dht.StageRetrying:
		if isCtxCancelled {
			asp.DHTState = log.Gray("cancelled")
			asp.WANState = log.Gray("cancelled")
		} else {
			asp.DHTState = spinnerChar
			asp.WANState = log.Green("searching " + spinnerChar)
		}
	case dht.StageError:
		if isCtxCancelled || errors.Is(dhtState.Err, context.Canceled) {
			asp.DHTState = log.Gray("cancelled")
			asp.WANState = log.Gray("cancelled")
		} else {
			asp.DHTState = log.Red(dhtState.Err.Error())
			asp.WANState = log.Red("failed (" + dhtState.Err.Error() + ")")
		}
	case dht.StageStopped:
		asp.DHTState = log.Green("inactive")
		asp.WANState = log.Green("stopped")
	}

	for peer, state := range pakeStates {
		peerID := peer.String()
		if len(peerID) >= 16 {
			peerID = peerID[:16]
		}

		if isCtxCancelled {
			asp.PeerStates[peerID] = log.Gray("cancelled")
		}

		asp.Peers = append(asp.Peers, peerID)
		switch state.Step {
		case node.PakeStepStart:
			asp.PeerStates[peerID] = "Started peer authentication"
		case node.PakeStepWaitingForKeyInformation:
			asp.PeerStates[peerID] = "Waiting for key information... " + spinnerChar
			if isCtxCancelled {
				asp.PeerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepCalculatingKeyInformation:
			asp.PeerStates[peerID] = "Calculating on key information... " + spinnerChar
			if isCtxCancelled {
				asp.PeerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepSendingKeyInformation:
			asp.PeerStates[peerID] = "Sending key information... " + spinnerChar
			if isCtxCancelled {
				asp.PeerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepWaitingForFinalKeyInformation:
			asp.PeerStates[peerID] = "Waiting for final key information... " + spinnerChar
			if isCtxCancelled {
				asp.PeerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepProvingAuthenticityToPeer:
			asp.PeerStates[peerID] = "Proving authenticity to peer... " + spinnerChar
			if isCtxCancelled {
				asp.PeerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepVerifyingProofFromPeer:
			asp.PeerStates[peerID] = "Verifying proof from peer... " + spinnerChar
			if isCtxCancelled {
				asp.PeerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepWaitingForFinalConfirmation:
			asp.PeerStates[peerID] = "Waiting for final confirmation... " + spinnerChar
			if isCtxCancelled {
				asp.PeerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepPeerAuthenticated:
			asp.PeerStates[peerID] = log.Green("Peer authenticated!")
		case node.PakeStepError:
			if state.Err != nil {
				asp.PeerStates[peerID] = log.Red("Peer authentication failed: " + state.Err.Error())
			} else {
				asp.PeerStates[peerID] = log.Red("Peer authentication failed")
			}
		default:
			asp.PeerStates[peerID] = log.Yellow(fmt.Sprintf("Unknown PAKE step: %d", state.Step))
		}
	}

	for peer, state := range hpStates {
		peerID := peer.String()
		if len(peerID) >= 16 {
			peerID = peerID[:16]
		}

		// give PAKE status precedence
		if _, found := asp.PeerStates[peerID]; found {
			continue
		}

		if isCtxCancelled {
			asp.PeerStates[peerID] = log.Gray("cancelled")
		}

		switch state.Stage {
		case node.HolePunchStageStarted:
			asp.PeerStates[peerID] = fmt.Sprintf("Hole punching NATs (attempt %d)... %s", state.Attempts, spinnerChar)
		case node.HolePunchStageSucceeded:
			asp.PeerStates[peerID] = log.Green("Hole punching succeeded!")
		case node.HolePunchStageFailed:
			asp.PeerStates[peerID] = log.Red(fmt.Sprintf("Hole punching failed (%s)", state.Err))
		}
	}

	sort.Strings(asp.Peers)

	return asp
}
