package mdns

import (
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	"github.com/libp2p/go-libp2p/p2p/discovery"
)

type Advertiser struct {
	*protocol
}

func NewAdvertiser(node *pcpnode.Node) *Advertiser {
	return &Advertiser{newProtocol(node)}
}

// Advertise broadcasts that we're providing data for the given code.
//
// TODO: NewMdnsService also polls for peers. This is quite chatty, so we could extract the server-only logic.
func (a *Advertiser) Advertise(chanID int) error {
	if err := a.ServiceStarted(); err != nil {
		return err
	}
	defer a.ServiceStopped()

	// TODO: Restart after 5 minutes for new DiscoveryIdentifier
	mdns, err := discovery.NewMdnsService(a.ServiceContext(), a, a.interval, a.DiscoveryIdentifier(chanID)) // keep in sync with discoverer
	if err != nil {
		return err
	}

	<-a.SigShutdown()

	return mdns.Close()
}

func (a *Advertiser) Shutdown() {
	a.Service.Shutdown()
}
