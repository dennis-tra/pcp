package wrap

import (
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

type Discoverer interface {
	NewMdnsService(peerhost host.Host, serviceTag string, notifee mdns.Notifee) mdns.Service
}

type Discovery struct{}

func (d Discovery) NewMdnsService(peerhost host.Host, serviceTag string, notifee mdns.Notifee) mdns.Service {
	return mdns.NewMdnsService(peerhost, serviceTag, notifee)
}
