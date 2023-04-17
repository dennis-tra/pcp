package mdns

import (
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/host"

	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/dennis-tra/pcp/pkg/service"
)

// These wrapped top level functions are here for testing purposes.
var (
	wraptime      wrap.Timer      = wrap.Time{}
	wrapdiscovery wrap.Discoverer = wrap.Discovery{}
)

var (
	// Timeout is the time until a new advertisement
	// with a potentially new discovery ID is started.
	Timeout = time.Minute

	// TruncateDuration represents the time slot to which
	// the current time is truncated.
	TruncateDuration = 5 * time.Minute
)

// protocol encapsulates the logic for discovering peers
// via multicast DNS in the local network.
type protocol struct {
	host.Host
	service.Service

	offset time.Duration
}

func newProtocol(h host.Host) *protocol {
	return &protocol{
		Host:    h,
		Service: service.New("mDNS"),
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

func (d *Discoverer) SetOffset(offset time.Duration) *Discoverer {
	d.offset = offset
	return d
}

// DiscoveryID returns the string, that we use to advertise
// via mDNS and the DHT. See chanID above for more information.
// Using UnixNano for testing.
func (p *protocol) DiscoveryID(chanID int) string {
	return fmt.Sprintf("/pcp/%d/%d", p.TimeSlotStart().UnixNano(), chanID)
}
