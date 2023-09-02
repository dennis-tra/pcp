package wrap

import "github.com/libp2p/go-libp2p/p2p/discovery/mdns"

type MDNSService interface {
	mdns.Service
}
