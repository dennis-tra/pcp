package discovery

import (
	"strconv"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/dennis-tra/pcp/internal/mock"
)

func TestID_DiscoveryIdentifier_returnsCorrect(t *testing.T) {
	ctrl := gomock.NewController(t)

	now := time.Now()

	m := mock.NewMockTimer(ctrl)
	m.EXPECT().Now().Return(now)
	wraptime = m

	did := ID{chanID: 333, role: RoleSender}
	id := did.DiscoveryID()

	unixNow := now.Truncate(TruncateDuration).UnixNano()
	assert.Equal(t, "/pcp/"+string(RoleSender)+"/"+strconv.Itoa(int(unixNow))+"/333", id)
}
