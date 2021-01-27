package app

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p/p2p/discovery"
)

type Discoverer interface {
	NewMdnsService(ctx context.Context, peerhost host.Host, interval time.Duration, serviceTag string) (discovery.Service, error)
}

type Discovery struct{}

func (a Discovery) NewMdnsService(ctx context.Context, peerhost host.Host, interval time.Duration, serviceTag string) (discovery.Service, error) {
	return discovery.NewMdnsService(ctx, peerhost, interval, serviceTag)
}
