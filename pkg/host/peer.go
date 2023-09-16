package host

import (
	"fmt"
	"sort"

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

		state := m.PeerStates[pID]
		switch state {
		case PeerStateConnected, PeerStateAuthenticating, PeerStateAuthenticated, PeerStateFailedAuthentication:
			out += fmt.Sprintf("  -> %s: %s\n", bold.Render(peerID)[:16], m.AuthProtocol.PakeStateStr(pID))
		}
	}

	return out
}
