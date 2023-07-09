package node

import (
	"github.com/libp2p/go-libp2p/core/event"
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
}
