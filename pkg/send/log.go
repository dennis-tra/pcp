package send

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gosuri/uilive"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/nat"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/dht"
	"github.com/dennis-tra/pcp/pkg/mdns"
	"github.com/dennis-tra/pcp/pkg/node"
	"github.com/dennis-tra/pcp/pkg/service"
)

type statusLogger struct {
	service.Service
	node   *Node
	writer *uilive.Writer
}

func newStatusLogger(n *Node) *statusLogger {
	return &statusLogger{
		Service: service.New("status-logger"),
		node:    n,
		writer:  uilive.New(),
	}
}

func (l *statusLogger) startLogging() {
	_ = l.ServiceStarted()
	defer l.ServiceStopped()

	log.Infoln("On the other machine run:")
	log.Infoln("\tpeercp receive", strings.Join(l.node.Words, "-"))

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	l.newLogStatus(log.SpinnerChars[0]).writeStatus(l.writer)
	_ = l.writer.Flush()

	for i := 0; ; i++ {
		i %= len(log.SpinnerChars)

		select {
		case <-l.SigShutdown():
			l.newLogStatus(log.SpinnerChars[i]).writeStatus(l.writer.Bypass())
			_ = l.writer.Flush()
			return
		case <-ticker.C:
			l.newLogStatus(log.SpinnerChars[i]).writeStatus(l.writer)
			_ = l.writer.Flush()
		}
	}
}

type logStatus struct {
	verbose           bool
	words             []string
	mdnsState         mdns.State
	dhtState          dht.AdvertiseState
	pakeStates        map[peer.ID]*node.PakeState
	hpStates          map[peer.ID]node.HolePunchState
	portMappings      []nat.Mapping
	spinnerChar       string
	ctxCancelled      bool
	relayFinderActive bool
	connTypes         map[peer.ID]connectionType
}

type connectionType string

const (
	connTypeDirect  connectionType = "direct"
	connTypeRelayed connectionType = "relay"
)

func (l *statusLogger) newLogStatus(spinnerChar string) *logStatus {
	var portMappings []nat.Mapping
	if l.node.NATManager.NAT() != nil {
		portMappings = l.node.NATManager.NAT().Mappings()
	}

	l.node.relayFinderActiveLk.RLock()
	rfa := l.node.relayFinderActive
	l.node.relayFinderActiveLk.RUnlock()

	pakeStates := l.node.PakeProtocol.PakeStates()

	connTypes := map[peer.ID]connectionType{}
	for p := range pakeStates {
		conns := l.node.Network().ConnsToPeer(p)
		connType := connTypeRelayed
		for _, conn := range conns {
			if !node.IsRelayAddress(conn.RemoteMultiaddr()) {
				connType = connTypeDirect
				break
			}
		}
		connTypes[p] = connType
	}

	return &logStatus{
		ctxCancelled:      l.node.ServiceContext().Err() == context.Canceled,
		spinnerChar:       spinnerChar,
		verbose:           l.node.Verbose,
		words:             l.node.Words,
		mdnsState:         l.node.mdnsAdvertiser.State(),
		dhtState:          l.node.dhtAdvertiser.State(),
		pakeStates:        pakeStates,
		connTypes:         connTypes,
		hpStates:          l.node.HolePunchStates(),
		portMappings:      portMappings,
		relayFinderActive: rfa,
	}
}

func (l *logStatus) writeStatus(writer io.Writer) {
	if l.verbose {
		fmt.Fprintf(writer, "Code:          %s\n", strings.Join(l.words, "-"))
		mdnsErr := ""
		if l.mdnsState.Err != nil {
			mdnsErr = log.Red(" (" + l.mdnsState.Err.Error() + ")")
		}
		fmt.Fprintf(writer, "mDNS:          %s%s\n", l.mdnsStateStr(), mdnsErr)
		dhtErr := ""
		if l.dhtState.Err != nil {
			dhtErr = log.Red(" (" + l.dhtState.Err.Error() + ")")
		}
		fmt.Fprintf(writer, "DHT:           %s%s\n", l.dhtStateStr(), dhtErr)
		fmt.Fprintf(writer, "Network:       %s\n", l.reachabilityStr())
		fmt.Fprintf(writer, "NAT (udp/tcp):\n")
		fmt.Fprintf(writer, "    Type:       %s\n", l.natStateStr())
		fmt.Fprintf(writer, "    Mappings:   %s\n", l.portMappingsStr())
		fmt.Fprintf(writer, "Multiaddresses\n")
		fmt.Fprintf(writer, "    private:    %s\n", l.privateAddrsStr())
		fmt.Fprintf(writer, "    public:     %s\n", l.publicAddrsStr())
		fmt.Fprintf(writer, "    relays:     %s\n", l.relayAddrsStr())
	}
	fmt.Fprint(writer, fmt.Sprintf("%s %s  %s %s\n", log.Bold("Local Network:"), l.mdnsStateStr(), log.Bold("Internet:"), l.dhtStateStr()))

	peers, peerStates := l.pakeStatesStr()
	hpStates := l.connectionStateStr()
	for _, p := range peers {
		fmt.Fprintf(writer, "  -> %s: %s %s\n", log.Bold(p), peerStates[p], hpStates[p])
	}
}

func (l *logStatus) mdnsStateStr() string {
	switch l.mdnsState.Stage {
	case mdns.StageIdle:
		return log.Gray("-")
	case mdns.StageRoaming:
		return log.Green("ready")
	case mdns.StageError:
		return log.Red("failed")
	case mdns.StageStopped:
		if l.ctxCancelled {
			return log.Gray("cancelled")
		}
		return log.Green("stopped")
	default:
		return log.Red(fmt.Sprintf("unknown stage: %s", l.mdnsState.Stage))
	}
}

func (l *logStatus) dhtStateStr() string {
	if l.dhtState.Stage == dht.StageIdle {
		return log.Gray("-")
	} else if l.dhtState.Stage == dht.StageError {
		return log.Red("failed")
	} else if l.ctxCancelled {
		return log.Gray("cancelled")
	}

	switch l.dhtState.Stage {
	case dht.StageBootstrapping:
		return l.spinnerChar + "(bootstrapping)"
	case dht.StageAnalyzingNetwork:
		if l.relayFinderActive {
			return l.spinnerChar + "(finding relays)"
		} else {
			return l.spinnerChar + "(analyzing network)"
		}
	case dht.StageProviding:
		return l.spinnerChar + "(writing to DHT)"
	case dht.StageRetrying:
		return l.spinnerChar + log.Yellow("(retry writing to DHT)")
	case dht.StageProvided:
		if l.dhtState.Reachability == network.ReachabilityPrivate && len(l.dhtState.RelayAddrs) == 0 {
			return log.Yellow("lost relay reservations")
		} else {
			return log.Green("ready")
		}
	case dht.StageStopped:
		return log.Green("stopped")
	default:
		return log.Red(fmt.Sprintf("unknown stage: %d", l.dhtState.Stage))
	}
}

func (l *logStatus) reachabilityStr() string {
	switch l.dhtState.Reachability {
	case network.ReachabilityUnknown:
		if l.dhtState.Stage == dht.StageIdle {
			return log.Gray("-")
		} else if l.ctxCancelled {
			return log.Gray("cancelled")
		} else {
			return l.spinnerChar
		}
	case network.ReachabilityPrivate:
		return log.Yellow(strings.ToLower(l.dhtState.Reachability.String()))
	case network.ReachabilityPublic:
		return log.Green(strings.ToLower(l.dhtState.Reachability.String()))
	default:
		return log.Red(fmt.Sprintf("unknown state: %s", l.dhtState.Reachability))
	}
}

func (l *logStatus) natStateStr() string {
	if l.dhtState.Stage == dht.StageIdle {
		return log.Gray("- / -")
	}
	udpType := l.natStr(l.dhtState.NATTypeUDP)
	tcpType := l.natStr(l.dhtState.NATTypeTCP)
	return fmt.Sprintf("%s / %s", udpType, tcpType)
}

func (l *logStatus) natStr(deviceType network.NATDeviceType) string {
	switch deviceType {
	case network.NATDeviceTypeUnknown:
		if l.dhtState.Reachability == network.ReachabilityPublic {
			return log.Gray("irrelevant")
		} else if l.ctxCancelled {
			return log.Gray("cancelled")
		} else {
			return l.spinnerChar
		}
	case network.NATDeviceTypeCone:
		return log.Green(strings.ToLower(l.dhtState.NATTypeUDP.String()))
	case network.NATDeviceTypeSymmetric:
		return log.Red(strings.ToLower(l.dhtState.NATTypeUDP.String()))
	default:
		return log.Red(fmt.Sprintf("unknown: %s", deviceType))
	}
}

func (l *logStatus) portMappingsStr() string {
	if l.dhtState.Stage == dht.StageIdle {
		return log.Gray("- / -")
	}

	pmsUDP := 0
	pmsTCP := 0
	for _, mapping := range l.portMappings {
		switch mapping.Protocol() {
		case "udp":
			pmsUDP += 1
		case "tcp":
			pmsTCP += 1
		}
	}

	pmsStr := ""
	if pmsUDP == 0 {
		pmsStr += log.Gray("0")
	} else {
		pmsStr += log.Green(strconv.Itoa(pmsUDP))
	}
	pmsStr += " / "
	if pmsTCP == 0 {
		pmsStr += log.Gray("0")
	} else {
		pmsStr += log.Green(strconv.Itoa(pmsTCP))
	}

	return pmsStr
}

func (l *logStatus) privateAddrsStr() string {
	if len(l.dhtState.PrivateAddrs) == 0 {
		return log.Red("0")
	} else {
		return log.Green(strconv.Itoa(len(l.dhtState.PrivateAddrs)))
	}
}

func (l *logStatus) publicAddrsStr() string {
	if l.dhtState.Stage == dht.StageIdle {
		return log.Gray("-")
	} else if len(l.dhtState.PublicAddrs) == 0 {
		if len(l.dhtState.RelayAddrs) > 0 {
			return log.Gray("0")
		} else if l.ctxCancelled {
			return log.Gray("cancelled")
		} else {
			return l.spinnerChar
		}
	} else {
		return log.Green(strconv.Itoa(len(l.dhtState.PublicAddrs)))
	}
}

func (l *logStatus) relayAddrsStr() string {
	if l.dhtState.Stage == dht.StageIdle {
		return log.Gray("-")
	} else if l.dhtState.Reachability == network.ReachabilityPublic {
		return log.Gray("irrelevant")
	} else if l.ctxCancelled {
		return log.Gray("cancelled")
	} else if l.relayFinderActive {
		if len(l.dhtState.RelayAddrs) > 0 {
			return log.Green(strconv.Itoa(len(l.dhtState.RelayAddrs)))
		}
		return l.spinnerChar
	}
	return log.Gray("-")
}

func (l *logStatus) connectionStateStr() map[string]string {
	states := map[string]string{}
	for p, connType := range l.connTypes {
		peerID := p.String()
		if len(peerID) >= 16 {
			peerID = peerID[:16]
		}

		if pakeState, found := l.pakeStates[p]; found {
			if pakeState.Step == node.PakeStepError {
				continue
			}
		}

		switch connType {
		case connTypeRelayed:
			hpStateStr := ""
			hpState, hpStateFound := l.hpStates[p]
			pakeState, pakeStateFound := l.pakeStates[p]
			if pakeStateFound && pakeState.Step == node.PakeStepError {
				hpStateStr = log.Gray("-")
			} else if !hpStateFound {
				hpStateStr = log.Gray(l.spinnerChar)
			} else {
				switch hpState.Stage {
				case node.HolePunchStageUnknown:
					hpStateStr = log.Gray(l.spinnerChar)
				case node.HolePunchStageStarted:
					hpStateStr = fmt.Sprintf("%s (attempt %d)", l.spinnerChar, hpState.Attempts)
				case node.HolePunchStageSucceeded:
					hpStateStr = log.Green("Succeeded!")
				case node.HolePunchStageFailed:
					hpStateStr = log.Red("Failed")
				default:
					hpStateStr = log.Yellow(fmt.Sprintf("unexpected: %d", hpState.Stage))
				}
			}

			states[peerID] = fmt.Sprintf("%s: %s", log.Bold("Hole punching"), hpStateStr)

		case connTypeDirect:
			_, found := l.hpStates[p]
			if found && l.dhtState.Reachability != network.ReachabilityPublic {
				states[peerID] = log.Bold("Hole punching: ") + log.Green("Succeeded!")
			} else {
				states[peerID] = log.Bold("Connection: ") + log.Green("Direct")
			}
		default:
			states[peerID] = log.Bold("Connection: ") + log.Yellow("unknown: "+string(connType))
		}
	}

	return states
}

func (l *logStatus) pakeStatesStr() ([]string, map[string]string) {
	var peers []string
	peerStates := map[string]string{}
	for peer, state := range l.pakeStates {
		peerID := peer.String()
		if len(peerID) >= 16 {
			peerID = peerID[:16]
		}

		if l.ctxCancelled {
			peerStates[peerID] = log.Gray("cancelled")
		}

		peers = append(peers, peerID)
		switch state.Step {
		case node.PakeStepStart:
			peerStates[peerID] = "Started peer authentication"
		case node.PakeStepWaitingForKeyInformation:
			peerStates[peerID] = "Waiting for key information... " + l.spinnerChar
			if l.ctxCancelled {
				peerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepCalculatingKeyInformation:
			peerStates[peerID] = "Calculating on key information... " + l.spinnerChar
			if l.ctxCancelled {
				peerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepSendingKeyInformation:
			peerStates[peerID] = "Sending key information... " + l.spinnerChar
			if l.ctxCancelled {
				peerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepWaitingForFinalKeyInformation:
			peerStates[peerID] = "Waiting for final key information... " + l.spinnerChar
			if l.ctxCancelled {
				peerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepExchangingSalt:
			peerStates[peerID] = "Exchanging password salt... " + l.spinnerChar
			if l.ctxCancelled {
				peerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepProvingAuthenticityToPeer:
			peerStates[peerID] = "Proving authenticity to peer... " + l.spinnerChar
			if l.ctxCancelled {
				peerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepVerifyingProofFromPeer:
			peerStates[peerID] = "Verifying proof from peer... " + l.spinnerChar
			if l.ctxCancelled {
				peerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepWaitingForFinalConfirmation:
			peerStates[peerID] = "Waiting for final confirmation... " + l.spinnerChar
			if l.ctxCancelled {
				peerStates[peerID] = log.Gray("cancelled")
			}
		case node.PakeStepPeerAuthenticated:
			peerStates[peerID] = log.Green("Peer authenticated!")
		case node.PakeStepError:
			if state.Err != nil {
				peerStates[peerID] = log.Red("Peer authentication failed: " + state.Err.Error())
			} else {
				peerStates[peerID] = log.Red("Peer authentication failed")
			}
		default:
			peerStates[peerID] = log.Yellow(fmt.Sprintf("Unknown PAKE step: %d", state.Step))
		}
	}

	sort.Strings(peers)
	return peers, peerStates
}
