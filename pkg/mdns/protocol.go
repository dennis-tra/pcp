package mdns

import (
	"time"

	"github.com/libp2p/go-libp2p/core/host"

	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/service"
)

// These wrapped top level functions are here for testing purposes.
var (
	wraptime      wrap.Timer      = wrap.Time{}
	wrapdiscovery wrap.Discoverer = wrap.Discovery{}
)

// Timeout is the time until a new advertisement
// with a potentially new discovery ID is started.
var Timeout = time.Minute

// protocol encapsulates the logic for discovering peers
// via multicast DNS in the local network.
type protocol struct {
	host.Host
	service.Service

	did discovery.ID
}

func newProtocol(h host.Host) *protocol {
	return &protocol{
		Host:    h,
		Service: service.New("mDNS"),
		did:     discovery.ID{},
	}
}

func (d *Discoverer) SetOffset(offset time.Duration) *Discoverer {
	d.did.SetOffset(offset)
	return d
}
