package mdns

import (
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p-core/host"

	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/dennis-tra/pcp/pkg/service"
)

var wraptime wrap.Timer = wrap.Time{}

// ConnThreshold represents the minimum number of bootstrap peers we need a connection to.
var (
	ConnThreshold    = 3
	TruncateDuration = 5 * time.Minute
)

// protocol encapsulates the logic for discovering peers
// via multicast DNS in the local network.
type protocol struct {
	host.Host
	*service.Service
	interval time.Duration

	offset time.Duration
}

func newProtocol(h host.Host) *protocol {
	return &protocol{
		Host:     h,
		interval: time.Second,
		Service:  service.New(),
	}
}

// TimeSlotStart returns the time when the current time slot started.f
func (p *protocol) TimeSlotStart() time.Time {
	return p.refTime().Truncate(TruncateDuration)
}

// refTime returns the reference time to calculate the time slot from.
func (p *protocol) refTime() time.Time {
	return wraptime.Now().Add(p.offset)
}

// DiscoveryID returns the string, that we use to advertise
// via mDNS and the DHT. See chanID above for more information.
// Using UnixNano for testing.
func (p *protocol) DiscoveryID(chanID int) string {
	return fmt.Sprintf("/pcp/%d/%d", p.TimeSlotStart().UnixNano(), chanID)
}
