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

func (h *Host) ViewPeerStates() string {
	out := ""

	var peerIDs []string
	for peerID := range h.PeerStates {
		peerIDs = append(peerIDs, peerID.String())
	}
	sort.Strings(peerIDs)

	style := lipgloss.NewStyle().Bold(true)

	for _, peerID := range peerIDs {
		pID, err := peer.Decode(peerID)
		if err != nil {
			panic(err)
		}

		state := h.PeerStates[pID]
		switch state {
		case PeerStateConnected:
			fallthrough
		case PeerStateAuthenticated:
			out += fmt.Sprintf("  -> %s: %s\n", style.Render(peerID)[:16], h.PakeProtocol.PakeStateStr(pID))
		}
	}

	return out
}
