package wrap

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	stddiscovery "github.com/libp2p/go-libp2p/p2p/discovery"
)

type Discoverer interface {
	NewMdnsService(ctx context.Context, peerhost host.Host, interval time.Duration, serviceTag string) (stddiscovery.Service, error)
}

type Discovery struct{}

func (d Discovery) NewMdnsService(ctx context.Context, peerhost host.Host, interval time.Duration, serviceTag string) (stddiscovery.Service, error) {
	return stddiscovery.NewMdnsService(ctx, peerhost, interval, serviceTag)
}
