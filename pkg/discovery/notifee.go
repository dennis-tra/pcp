package discovery

import (
	"github.com/libp2p/go-libp2p/core/peer"
)

type Notifee interface {
	HandlePeerFound(pi peer.AddrInfo)
}
