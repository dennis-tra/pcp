package receive

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gosuri/uilive"
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

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for i := 0; ; i++ {
		i %= len(log.SpinnerChars)

		select {
		case <-l.SigShutdown():
			l.newLogStatus(log.SpinnerChars[i]).writeStatus(l.writer)
			_ = l.writer.Flush()
			return
		case <-ticker.C:
			l.newLogStatus(log.SpinnerChars[i]).writeStatus(l.writer)
			_ = l.writer.Flush()
		}
	}
}

type logStatus struct {
	verbose      bool
	words        []string
	mdnsState    mdns.State
	dhtState     dht.DiscoverState
	pakeStates   map[peer.ID]*node.PakeState
	hpStates     map[peer.ID]node.HolePunchState
	portMappings []nat.Mapping
	spinnerChar  string
	ctxCancelled bool
}

func (l *statusLogger) newLogStatus(spinnerChar string) *logStatus {
	var portMappings []nat.Mapping
	if l.node.NATManager.NAT() != nil {
		portMappings = l.node.NATManager.NAT().Mappings()
	}

	return &logStatus{
		ctxCancelled: l.node.ServiceContext().Err() == context.Canceled,
		spinnerChar:  spinnerChar,
		verbose:      l.node.verbose,
		words:        l.node.Words,
		mdnsState:    l.node.mdnsDiscoverer.State(),
		dhtState:     l.node.dhtDiscoverer.State(),
		pakeStates:   l.node.PakeProtocol.PakeStates(),
		hpStates:     l.node.HolePunchStates(),
		portMappings: portMappings,
	}
}

func (l *logStatus) writeStatus(writer io.Writer) {
	if l.verbose {
		fmt.Fprintf(writer, "Code:          %s\n", strings.Join(l.words, "-"))
		fmt.Fprintf(writer, "mDNS:          %s\n", l.mdnsStateStr())
		fmt.Fprintf(writer, "DHT:           %s\n", l.dhtStateStr())
		fmt.Fprintf(writer, "NAT (udp/tcp):\n")
		fmt.Fprintf(writer, "   Mappings:   %s\n", l.portMappingsStr())
		fmt.Fprintf(writer, "Multiaddresses\n")
		fmt.Fprintf(writer, "   private:    %s\n", l.privateAddrsStr())
		fmt.Fprintf(writer, "   public:     %s\n", l.publicAddrsStr())
	}
	fmt.Fprint(writer, fmt.Sprintf("%s %s\t%s %s\n", log.Bold("Local Network:"), l.lanStateStr(), log.Bold("Internet:"), l.wanStateStr()))

	peers, peerStates := l.peerStates()
	for _, p := range peers {
		fmt.Fprintf(writer, "  -> %s: %s\n", log.Bold(p), peerStates[p])
	}
}

func (l *logStatus) mdnsStateStr() string {
	switch l.mdnsState.Stage {
	case mdns.StageIdle:
		return log.Gray("-")
	case mdns.StageRoaming:
		return log.Green("active")
	case mdns.StageError:
		return log.Red(l.mdnsState.Err.Error())
	case mdns.StageStopped:
		return log.Green("inactive")
	default:
		return log.Red(fmt.Sprintf("unknown stage: %s", l.mdnsState.Stage))
	}
}

func (l *logStatus) lanStateStr() string {
	switch l.mdnsState.Stage {
	case mdns.StageIdle:
		return log.Green("-")
	case mdns.StageRoaming:
		return log.Green("searching " + l.spinnerChar)
	case mdns.StageError:
		return log.Red("failed")
	case mdns.StageStopped:
		if l.ctxCancelled {
			return log.Gray("cancelled")
		}
		return log.Green("stopped")
	default:
		return log.Gray("-")
	}
}

func (l *logStatus) dhtStateStr() string {
	switch l.dhtState.Stage {
	case dht.StageIdle:
		return log.Gray("-")
	case dht.StageStopped:
		return log.Green("stopped")
	}

	if l.ctxCancelled {
		return log.Gray("cancelled")
	}

	switch l.dhtState.Stage {
	case dht.StageBootstrapping:
		return l.spinnerChar + "(bootstrapping)"
	case dht.StageWaitingForPublicAddrs:
		return l.spinnerChar + "(detecting public addresses)"
	case dht.StageLookup:
		fallthrough
	case dht.StageRetrying:
		return log.Green("searching " + l.spinnerChar)
	case dht.StageError:
		return log.Red("failed (" + l.dhtState.Err.Error() + ")")
	default:
		return log.Red(fmt.Sprintf("unknown stage: %d", l.dhtState.Stage))
	}
}

func (l *logStatus) wanStateStr() string {
	if l.dhtState.Stage == dht.StageIdle {
		return log.Gray("-")
	} else if l.ctxCancelled {
		return log.Gray("cancelled")
	}

	switch l.dhtState.Stage {
	case dht.StageBootstrapping:
		return l.spinnerChar + "(bootstrapping)"
	case dht.StageWaitingForPublicAddrs:
		return l.spinnerChar + "(detecting public addresses)"
	case dht.StageLookup:
		fallthrough
	case dht.StageRetrying:
		return log.Green("searching " + l.spinnerChar)
	case dht.StageStopped:
		return log.Green("stopped")
	case dht.StageError:
		return log.Red(l.dhtState.Err.Error())
	default:
		return log.Red(fmt.Sprintf("unknown stage: %d", l.dhtState.Stage))
	}
}

func (l *logStatus) portMappingsStr() string {
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
	if len(l.dhtState.PublicAddrs) == 0 {
		if l.ctxCancelled {
			return log.Gray("cancelled")
		} else {
			return l.spinnerChar
		}
	} else {
		return log.Green(strconv.Itoa(len(l.dhtState.PublicAddrs)))
	}
}

func (l *logStatus) peerStates() ([]string, map[string]string) {
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

	for peer, state := range l.hpStates {
		peerID := peer.String()
		if len(peerID) >= 16 {
			peerID = peerID[:16]
		}

		// give PAKE status precedence
		if _, found := peerStates[peerID]; found {
			continue
		}

		if l.ctxCancelled {
			peerStates[peerID] = log.Gray("cancelled")
		}

		switch state.Stage {
		case node.HolePunchStageStarted:
			peerStates[peerID] = fmt.Sprintf("Hole punching NATs (attempt %d)... %s", state.Attempts, l.spinnerChar)
		case node.HolePunchStageSucceeded:
			peerStates[peerID] = log.Green("Hole punching succeeded!")
		case node.HolePunchStageFailed:
			peerStates[peerID] = log.Red(fmt.Sprintf("Hole punching failed (%s)", state.Err))
		}
	}

	sort.Strings(peers)

	return peers, peerStates
}
