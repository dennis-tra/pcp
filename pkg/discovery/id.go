package discovery

import (
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/dennis-tra/pcp/internal/wrap"
)

type Role string

const (
	RoleUndefined Role = "undefined"
	RoleSender    Role = "sender"
	RoleReceiver  Role = "receiver"
)

func (r Role) Opposite() Role {
	switch r {
	case RoleSender:
		return RoleReceiver
	case RoleReceiver:
		return RoleSender
	case RoleUndefined:
		return RoleUndefined
	default:
		panic(r)
	}
}

var (
	// TruncateDuration represents the time slot to which the current time is truncated.
	TruncateDuration = 1 * time.Minute

	// wraptime wraps the time package for testing
	wraptime wrap.Timer = wrap.Time{}
)

type ID struct {
	chanID int
	offset time.Duration
	role   Role
}

func NewID(chanID int) *ID {
	return &ID{chanID: chanID, role: RoleUndefined}
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
func (id *ID) DiscoveryID() string {
	return fmt.Sprintf("/pcp/%s/%d/%d", id.role, id.TimeSlotStart().UnixNano(), id.chanID)
}

func (id *ID) SetOffset(offset time.Duration) *ID {
	id.offset = offset
	return id
}

func (id *ID) SetRole(role Role) *ID {
	id.role = role
	return id
}

// ContentID hashes the given discoveryID and returns the corresponding CID.
func (id *ID) ContentID() (cid.Cid, error) {
	h, err := mh.Sum([]byte(id.DiscoveryID()), mh.SHA2_256, -1)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(cid.Raw, h), nil
}
