package wrap

import "github.com/libp2p/go-libp2p/core/peerstore"

type Peerstore interface {
	peerstore.Peerstore
}
