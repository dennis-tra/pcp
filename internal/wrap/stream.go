package wrap

import (
	"github.com/libp2p/go-libp2p/core/network"
)

type Stream interface {
	network.Stream
}
