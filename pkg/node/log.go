package node

import (
	"github.com/libp2p/go-libp2p/core/event"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/dennis-tra/pcp/internal/log"
)

func (n *Node) debugLogEvents() {
	evtTypes := []interface{}{
		new(event.EvtLocalReachabilityChanged),
		new(event.EvtNATDeviceTypeChanged),
		new(event.EvtLocalAddressesUpdated),
	}
	sub, err := n.EventBus().Subscribe(evtTypes)
	if err != nil {
		return
	}
	defer sub.Close()

	for {
		var (
			e    interface{}
			more bool
		)

		select {
		case <-n.ServiceContext().Done():
			return
		case e, more = <-sub.Out():
			if !more {
				return
			}
		}
		switch evt := e.(type) {
		case event.EvtLocalReachabilityChanged:
			log.Debugln("New reachability", evt.Reachability.String())
		case event.EvtNATDeviceTypeChanged:
			log.Debugf("New NAT Device Type %s=%s\n", evt.NatDeviceType, evt.TransportProtocol)
		case event.EvtLocalAddressesUpdated:
			publicAddresses := 0
			privateAddresses := 0
			relayAddresses := 0
			for _, update := range evt.Current {
				if _, err := update.Address.ValueForProtocol(ma.P_CIRCUIT); err == nil {
					relayAddresses += 1
				} else if manet.IsPublicAddr(update.Address) {
					publicAddresses += 1
				} else if manet.IsPrivateAddr(update.Address) {
					privateAddresses += 1
				}
			}
			log.Debugf("New Addresses private=%d public=%d relay=%d\n", privateAddresses, publicAddresses, relayAddresses)
		}
	}
}
