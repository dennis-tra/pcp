package discovery

import (
	"github.com/libp2p/go-libp2p-core/peer"
)

type PeerHandler interface {
	HandlePeer(info peer.AddrInfo)
}

type Discoverer interface {
	Discover(identifier string, handler PeerHandler) error
	Shutdown()
}

type Advertiser interface {
	Advertise(code string) error
	Shutdown()
}
