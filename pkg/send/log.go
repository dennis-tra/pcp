package send

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dennis-tra/pcp/pkg/node"

	"github.com/dennis-tra/pcp/pkg/dht"

	"github.com/dennis-tra/pcp/pkg/mdns"

	"github.com/libp2p/go-libp2p/core/network"

	"github.com/dennis-tra/pcp/internal/log"
)

var spinnerChars = []string{"⠋ ", "⠙ ", "⠹ ", "⠸ ", "⠼ ", "⠴ ", "⠦ ", "⠧ ", "⠇ ", "⠏ "}

func (n *Node) printStatus(stop chan struct{}) {
	n.logLoopWg.Add(1)
	defer n.logLoopWg.Done()

	log.Infoln("On the other machine run:")
	log.Infoln("\tpeercp receive", strings.Join(n.Words, "-"))

	eraseFn := log.AdvertiseStatus(n.advertiseStatus(spinnerChars[0]), n.verbose)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for i := 0; ; i++ {
		spinnerChar := spinnerChars[i%len(spinnerChars)]
		select {
		case <-stop:
			eraseFn()
			eraseFn = log.AdvertiseStatus(n.advertiseStatus(spinnerChar), n.verbose)
			return
		case <-n.ServiceContext().Done():
			return
		case <-ticker.C:
			eraseFn()
			eraseFn = log.AdvertiseStatus(n.advertiseStatus(spinnerChar), n.verbose)
		}
	}
}

func (n *Node) advertiseStatus(spinnerChar string) log.AdvertiseStatusParams {
	isCtxCancelled := n.ServiceContext().Err() == context.Canceled

	mdnsState := n.mdnsAdvertiser.State()
	dhtState := n.dhtAdvertiser.State()
	pakeStates := n.PakeProtocol.PakeStates()

	asp := log.AdvertiseStatusParams{
		Code:         strings.Join(n.Words, "-"),
		LANState:     log.Gray("-"),
		MDNSState:    log.Gray("-"),
		Reachability: log.Gray("-"),
		RelayAddrs:   log.Gray("-"),
		PakePeers:    []string{},
		PakeStates:   map[string]string{},
	}

	switch mdnsState.Stage {
	case mdns.StageIdle:
		asp.MDNSState = log.Gray("-")
		asp.LANState = log.Green("-")
	case mdns.StageAdvertising:
		asp.MDNSState = log.Green("active")
		asp.LANState = log.Green("ready")
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
	case dht.StageAnalyzingNetwork:
		if isCtxCancelled {
			asp.DHTState = log.Gray("cancelled")
			asp.WANState = log.Gray("cancelled")
		} else if n.relayFinderActive {
			asp.DHTState = spinnerChar + "(finding relays)"
			asp.WANState = spinnerChar + "(finding relays)"
		} else {
			asp.DHTState = spinnerChar + "(analyzing network)"
			asp.WANState = spinnerChar + "(analyzing network)"
		}
	case dht.StageProviding:
		if isCtxCancelled {
			asp.DHTState = log.Gray("cancelled")
			asp.WANState = log.Gray("cancelled")
		} else {
			asp.DHTState = spinnerChar + "(providing)"
			asp.WANState = spinnerChar + "(writing to DHT)"
		}
	case dht.StageRetrying:
		if isCtxCancelled {
			asp.DHTState = log.Gray("cancelled")
			asp.WANState = log.Gray("cancelled")
		} else {
			asp.DHTState = spinnerChar + "(retrying)"
			asp.WANState = spinnerChar + log.Yellow("(retry writing to DHT)")
		}
	case dht.StageProvided:
		asp.DHTState = log.Green("active")
		asp.WANState = log.Green("ready")
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

	switch dhtState.Reachability {
	case network.ReachabilityUnknown:
		if isCtxCancelled {
			asp.Reachability = log.Gray("cancelled")
		} else {
			asp.Reachability = spinnerChar
		}
	case network.ReachabilityPrivate:
		asp.Reachability = log.Yellow(strings.ToLower(dhtState.Reachability.String()))
	case network.ReachabilityPublic:
		asp.Reachability = log.Green(strings.ToLower(dhtState.Reachability.String()))
	}

	asp.NATState = ""
	switch dhtState.NATTypeUDP {
	case network.NATDeviceTypeUnknown:
		if dhtState.Reachability == network.ReachabilityPublic {
			asp.NATState += log.Gray("irrelevant")
		} else if isCtxCancelled {
			asp.NATState = log.Gray("cancelled")
		} else {
			asp.NATState += spinnerChar
		}
	case network.NATDeviceTypeCone:
		asp.NATState += log.Green(strings.ToLower(dhtState.NATTypeUDP.String()))
	case network.NATDeviceTypeSymmetric:
		asp.NATState += log.Red(strings.ToLower(dhtState.NATTypeUDP.String()))
	}
	asp.NATState += " / "
	switch dhtState.NATTypeTCP {
	case network.NATDeviceTypeUnknown:
		if dhtState.Reachability == network.ReachabilityPublic {
			asp.NATState += log.Gray("irrelevant")
		} else if isCtxCancelled {
			asp.NATState = log.Gray("cancelled")
		} else {
			asp.NATState += spinnerChar + " "
		}
	case network.NATDeviceTypeCone:
		asp.NATState += log.Green(strings.ToLower(dhtState.NATTypeTCP.String()))
	case network.NATDeviceTypeSymmetric:
		asp.NATState += log.Red(strings.ToLower(dhtState.NATTypeTCP.String()))
	}

	if dhtState.Reachability == network.ReachabilityPublic {
		asp.RelayAddrs = log.Gray("irrelevant")
	} else if isCtxCancelled {
		asp.RelayAddrs = log.Gray("cancelled")
	} else if n.relayFinderActive {
		asp.RelayAddrs = spinnerChar
		if len(dhtState.RelayAddrs) > 0 {
			asp.RelayAddrs = log.Green(strconv.Itoa(len(dhtState.RelayAddrs)))
		}
	}

	if len(dhtState.PrivateAddrs) == 0 {
		asp.PrivateAddrs = log.Red("0")
	} else {
		asp.PrivateAddrs = log.Green(strconv.Itoa(len(dhtState.PrivateAddrs)))
	}

	if len(dhtState.PublicAddrs) == 0 {
		if len(dhtState.RelayAddrs) > 0 {
			asp.PublicAddrs = log.Gray("0")
		} else if isCtxCancelled {
			asp.PublicAddrs = log.Gray("cancelled")
		} else {
			asp.PublicAddrs = spinnerChar
		}
	} else {
		asp.PublicAddrs = log.Green(strconv.Itoa(len(dhtState.PublicAddrs)))
	}

	for peer, state := range pakeStates {
		peerID := peer.String()
		if len(peerID) >= 16 {
			peerID = peerID[:16]
		}

		if isCtxCancelled {
			asp.PakeStates[peerID] = log.Gray("cancelled")
		}

		asp.PakePeers = append(asp.PakePeers, peerID)
		switch state.Step {
		case node.PakeStepStart:
			asp.PakeStates[peerID] = "Started peer authentication"
		case node.PakeStepWaitingForKeyInformation:
			asp.PakeStates[peerID] = "Waiting for key information... " + spinnerChar
			if isCtxCancelled {
				asp.PakeStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepCalculatingKeyInformation:
			asp.PakeStates[peerID] = "Calculating on key information... " + spinnerChar
			if isCtxCancelled {
				asp.PakeStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepSendingKeyInformation:
			asp.PakeStates[peerID] = "Sending key information... " + spinnerChar
			if isCtxCancelled {
				asp.PakeStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepWaitingForFinalKeyInformation:
			asp.PakeStates[peerID] = "Waiting for final key information... " + spinnerChar
			if isCtxCancelled {
				asp.PakeStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepProvingAuthenticityToPeer:
			asp.PakeStates[peerID] = "Proving authenticity to peer... " + spinnerChar
			if isCtxCancelled {
				asp.PakeStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepVerifyingProofFromPeer:
			asp.PakeStates[peerID] = "Verifying proof from peer... " + spinnerChar
			if isCtxCancelled {
				asp.PakeStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepWaitingForFinalConfirmation:
			asp.PakeStates[peerID] = "Waiting for final confirmation... " + spinnerChar
			if isCtxCancelled {
				asp.PakeStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepPeerAuthenticated:
			asp.PakeStates[peerID] = log.Green("Peer authenticated!")
		case node.PakeStepError:
			if state.Err != nil {
				asp.PakeStates[peerID] = log.Red("Peer authentication failed: " + state.Err.Error())
			} else {
				asp.PakeStates[peerID] = log.Red("Peer authentication failed")
			}
		default:
			asp.PakeStates[peerID] = log.Yellow(fmt.Sprintf("Unknown PAKE step: %d", state.Step))
		}
	}
	sort.Strings(asp.PakePeers)

	return asp
}
