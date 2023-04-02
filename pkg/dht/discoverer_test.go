package dht

import (
	"context"
	"sync"
	"testing"
	"time"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dennis-tra/pcp/internal/mock"
)

func TestDiscoverer_Discover_happyPath(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	dht := mock.NewMockIpfsDHT(ctrl)
	d := NewDiscoverer(local, dht)

	piChan := make(chan peer.AddrInfo)
	dht.EXPECT().
		FindProvidersAsync(gomock.Any(), gomock.Any(), 100).
		DoAndReturn(func(ctx context.Context, cID cid.Cid, count int) <-chan peer.AddrInfo {
			go func() {
				<-ctx.Done()
				close(piChan)
			}()
			return piChan
		})

	var wg sync.WaitGroup
	wg.Add(2)

	provider1, err := net.GenPeer()
	require.NoError(t, err)

	provider2, err := net.GenPeer()
	require.NoError(t, err)

	go func() {
		piChan <- peer.AddrInfo{
			ID:    provider1.ID(),
			Addrs: provider1.Addrs(),
		}

		piChan <- peer.AddrInfo{
			ID:    provider2.ID(),
			Addrs: provider2.Addrs(),
		}
	}()

	handler := func(pi peer.AddrInfo) {
		assert.True(t, pi.ID == provider1.ID() || pi.ID == provider2.ID())
		wg.Done()
	}

	go func() {
		wg.Wait()
		d.Shutdown()
	}()

	err = d.Discover(333, handler)
	assert.NoError(t, err)
}

func TestDiscoverer_Discover_reschedulesFindProvider(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	dht := mock.NewMockIpfsDHT(ctrl)
	d := NewDiscoverer(local, dht)

	var wg sync.WaitGroup
	wg.Add(5)

	dht.EXPECT().
		FindProvidersAsync(gomock.Any(), gomock.Any(), 100).
		DoAndReturn(func(ctx context.Context, cID cid.Cid, count int) <-chan peer.AddrInfo {
			piChan := make(chan peer.AddrInfo)
			go close(piChan)
			wg.Done()
			return piChan
		}).Times(5)

	go func() {
		wg.Wait()
		d.Shutdown()
	}()

	err := d.Discover(333, nil)
	assert.NoError(t, err)
}

func TestDiscoverer_Discover_callsFindProviderWithMutatingDiscoveryIDs(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	TruncateDuration = 10 * time.Millisecond

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	dht := mock.NewMockIpfsDHT(ctrl)
	d := NewDiscoverer(local, dht)

	var wg sync.WaitGroup
	wg.Add(2)

	var cIDs []string
	dht.EXPECT().
		FindProvidersAsync(gomock.Any(), gomock.Any(), 100).
		DoAndReturn(func(ctx context.Context, cID cid.Cid, count int) <-chan peer.AddrInfo {
			cIDs = append(cIDs, cID.String())
			piChan := make(chan peer.AddrInfo)
			go func() {
				time.Sleep(2 * TruncateDuration)
				close(piChan)
			}()
			wg.Done()
			return piChan
		}).Times(2)

	go func() {
		wg.Wait()
		d.Shutdown()
	}()

	err := d.Discover(333, nil)
	assert.NoError(t, err)

	assert.NotEqual(t, cIDs[0], cIDs[1])
}

func TestTimeCriticalDiscoverer_Discover_restartAsSoonAsCurrentTimeSlotIsExpired(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping time critical test") // They are flaky on GitHub actions
	}

	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	provideTimeout = 20 * time.Millisecond

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	dht := mock.NewMockIpfsDHT(ctrl)
	d := NewDiscoverer(local, dht)

	rounds := 5
	var wg sync.WaitGroup
	wg.Add(rounds)

	dht.EXPECT().
		FindProvidersAsync(gomock.Any(), gomock.Any(), 100).
		DoAndReturn(func(ctx context.Context, cID cid.Cid, count int) <-chan peer.AddrInfo {
			piChan := make(chan peer.AddrInfo)
			go func() {
				<-ctx.Done()
				close(piChan)
			}()
			wg.Done()
			return piChan
		}).Times(rounds)

	go func() {
		wg.Wait()
		d.Shutdown()
	}()

	start := time.Now()
	err := d.Discover(333, nil)
	end := time.Now()

	assert.NoError(t, err)

	// Only 4 because last round is immediately termianated by d.Shutdown()
	assert.InDelta(t, 4*provideTimeout, end.Sub(start), float64(provideTimeout))
}

func TestDiscoverer_SetOffset(t *testing.T) {
	net := mocknet.New()
	local, err := net.GenPeer()
	require.NoError(t, err)
	d := NewDiscoverer(local, nil)
	id1 := d.DiscoveryID(333)
	d.SetOffset(TruncateDuration * 3)
	id2 := d.DiscoveryID(333)
	assert.NotEqual(t, id1, id2)
}
