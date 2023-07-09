package wrap

import (
	"github.com/libp2p/go-libp2p/core/network"
)

type Conn interface {
	network.Conn
}
