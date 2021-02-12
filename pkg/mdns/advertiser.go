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
func (a *Advertiser) Advertise(code string) error {
	if err := a.ServiceStarted(); err != nil {
		return err
	}
	defer a.ServiceStopped()

	mdns, err := discovery.NewMdnsService(a.ServiceContext(), a, a.interval, code) // keep in sync with discoverer
	if err != nil {
		return err
	}

	<-a.SigShutdown()

	return mdns.Close()
}

func (a *Advertiser) Shutdown() {
	a.Service.Shutdown()
}
