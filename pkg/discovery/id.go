package discovery

import (
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/dennis-tra/pcp/internal/wrap"
)

var (
	// TruncateDuration represents the time slot to which the current time is truncated.
	TruncateDuration = 5 * time.Minute

	// wraptime wraps the time package for testing
	wraptime wrap.Timer = wrap.Time{}
)

type ID struct {
	offset time.Duration
}

// TimeSlotStart returns the time when the current time slot started.
func (id *ID) TimeSlotStart() time.Time {
	return id.refTime().Truncate(TruncateDuration)
}

// refTime returns the reference time to calculate the time slot from.
func (id *ID) refTime() time.Time {
	return wraptime.Now().Add(id.offset)
}

// DiscoveryID returns the string, that we use to advertise
// via mDNS and the DHT. See chanID above for more information.
// Using UnixNano for testing.
func (id *ID) DiscoveryID(chanID int) string {
	return fmt.Sprintf("/pcp/%d/%d", id.TimeSlotStart().UnixNano(), chanID)
}

func (id *ID) SetOffset(offset time.Duration) {
	id.offset = offset
}

// ContentID hashes the given discoveryID and returns the corresponding CID.
func (id *ID) ContentID(discoveryID string) (cid.Cid, error) {
	h, err := mh.Sum([]byte(discoveryID), mh.SHA2_256, -1)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(cid.Raw, h), nil
}
