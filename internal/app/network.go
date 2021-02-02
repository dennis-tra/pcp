package app

import (
	"github.com/libp2p/go-libp2p-core/network"
)

type Streamer interface {
	network.Stream
}

type Conner interface {
	network.Conn
}
