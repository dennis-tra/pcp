package mdns

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/discovery"

	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/libp2p/go-libp2p/core/host"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dennis-tra/pcp/internal/mock"
)

func setup(t *testing.T) (*gomock.Controller, host.Host, func(t *testing.T)) {
	wrapdiscovery = wrap.Discovery{}
	wraptime = wrap.Time{}

	ctrl := gomock.NewController(t)

	net := mocknet.New(context.Background())

	tmpTruncateDuration := TruncateDuration
	tmpInterval := Interval
	tmpTimeout := Timeout

	local, err := net.GenPeer()
	require.NoError(t, err)

	return ctrl, local, func(t *testing.T) {
		ctrl.Finish()

		TruncateDuration = tmpTruncateDuration
		Interval = tmpInterval
		Timeout = tmpTimeout

		wrapdiscovery = wrap.Discovery{}
		wraptime = wrap.Time{}
	}
}

func TestAdvertiser_Advertise(t *testing.T) {
	ctrl, local, teardown := setup(t)
	defer teardown(t)

	d := mock.NewMockDiscoverer(ctrl)
	wrapdiscovery = d

	a := NewAdvertiser(local)

	var wg sync.WaitGroup
	wg.Add(1)

	d.EXPECT().
		NewMdnsService(gomock.Any(), a, Interval, gomock.Any()).
		DoAndReturn(func(ctx context.Context, peerhost host.Host, interval time.Duration, serviceTag string) (discovery.Service, error) {
			wg.Done()
			return DummyMDNSService{}, nil
		}).
		Times(1)

	go func() {
		err := a.Advertise(333)
		assert.NoError(t, err)
		wg.Done()
	}()
	wg.Wait()
	wg.Add(1)

	a.Shutdown()
	wg.Wait()
}

func TestAdvertiser_Advertise_multipleTimes(t *testing.T) {
	ctrl, local, teardown := setup(t)
	defer teardown(t)

	Timeout = 20 * time.Millisecond

	d := mock.NewMockDiscoverer(ctrl)
	wrapdiscovery = d

	a := NewAdvertiser(local)

	var wg sync.WaitGroup
	wg.Add(5)

	d.EXPECT().
		NewMdnsService(gomock.Any(), a, Interval, gomock.Any()).
		DoAndReturn(func(ctx context.Context, peerhost host.Host, interval time.Duration, serviceTag string) (discovery.Service, error) {
			wg.Done()
			return DummyMDNSService{}, nil
		}).
		Times(5)

	go func() {
		err := a.Advertise(333)
		assert.NoError(t, err)
		wg.Done()
	}()
	wg.Wait()
	wg.Add(1)

	a.Shutdown()
	wg.Wait()
}

type DummyMDNSService struct{}

func (mdns DummyMDNSService) Close() error {
	return nil
}
func (mdns DummyMDNSService) RegisterNotifee(notifee discovery.Notifee)   {}
func (mdns DummyMDNSService) UnregisterNotifee(notifee discovery.Notifee) {}
