package mdns

import (
	"context"
	"time"

	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	"github.com/libp2p/go-libp2p/p2p/discovery"
)

type Advertiser struct {
	*protocol
	service discovery.Service
}

func newProtocol(node *pcpnode.Node) *protocol {
	m := &protocol{
		Node:     node,
		interval: time.Second,
	}
	return m
}

func NewAdvertiser(node *pcpnode.Node) *Advertiser {
	return &Advertiser{
		protocol: newProtocol(node),
		service:  nil,
	}
}

// Advertise broadcasts that we're providing data for the given code.
//
// TODO: NewMdnsService also polls for peers. This is quite chatty, so we could extract the server-only logic.
func (a *Advertiser) Advertise(ctx context.Context, code string) error {
	if a.service != nil {
		return nil
	}

	mdns, err := discovery.NewMdnsService(ctx, a, a.interval, ServicePrefix+"/"+code) // keep in sync with discoverer
	if err != nil {
		return err
	}
	a.service = mdns

	return nil
}

// Stop stops broadcasting multicast DNS messages.
func (a *Advertiser) Stop() error {
	if a.service == nil {
		return nil
	}

	a.service = nil

	err := a.service.Close()
	if err != nil {
		return err
	}

	return nil
}
