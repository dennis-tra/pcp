package host

import (
	"fmt"
	"sort"

	"github.com/dennis-tra/pcp/pkg/tui"

	"github.com/charmbracelet/lipgloss"
	"github.com/libp2p/go-libp2p/core/peer"
)

type PeerState string

const (
	PeerStateNotConnected         PeerState = "NotConnected"
	PeerStateConnecting           PeerState = "Connecting"
	PeerStateConnected            PeerState = "Connected"
	PeerStateAuthenticating       PeerState = "Authenticating"
	PeerStateAuthenticated        PeerState = "Authenticated"
	PeerStateFailedConnecting     PeerState = "FailedConnecting"
	PeerStateFailedAuthentication PeerState = "FailedAuthentication"
)

func (m *Model) ViewPeerStates() string {
	out := ""

	var peerIDs []string
	for peerID := range m.PeerStates {
		peerIDs = append(peerIDs, peerID.String())
	}
	sort.Strings(peerIDs)

	bold := lipgloss.NewStyle().Bold(true)

	for _, peerID := range peerIDs {
		pID, err := peer.Decode(peerID)
		if err != nil {
			log.WithError(err).WithField("peerID", peerID).Warnln("failed parsing peerID")
			continue
		}

		hpStr := ""
		if hpState, found := m.hpStates[pID]; found {
			switch hpState.Stage {
			case HolePunchStageUnknown:
			case HolePunchStageStarted:
				hpStr += fmt.Sprintf("| hole punching attempt %d %s", hpState.Attempts+1, m.spinner.View())
			case HolePunchStageSucceeded:
				hpStr += "| " + tui.Green.Render(fmt.Sprintf("hole punched!"))
			case HolePunchStageFailed:
				hpStr += "| " + tui.Red.Render(fmt.Sprintf("hole punch failed: %s", hpState.Err))
			}
		} else if pID == m.authedPeer && m.state == HostStateWaitingForDirectConn {
			hpStr = "| " + tui.Gray.Render(fmt.Sprintf("hole punching %s", m.spinner.View()))
		}

		state := m.PeerStates[pID]
		switch state {
		case PeerStateConnected, PeerStateAuthenticating, PeerStateAuthenticated, PeerStateFailedAuthentication:
			out += fmt.Sprintf("  -> %s: %s %s\n", bold.Render(peerID)[:16], m.AuthProt.AuthStateStr(pID), hpStr)
		}
	}

	return out
}
