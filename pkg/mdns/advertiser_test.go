package mdns

import (
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dennis-tra/pcp/internal/mock"
	"github.com/dennis-tra/pcp/internal/wrap"
)

func setup(t *testing.T) (*gomock.Controller, host.Host) {
	wrapdiscovery = wrap.Discovery{}
	wraptime = wrap.Time{}

	ctrl := gomock.NewController(t)

	net := mocknet.New()

	tmpTruncateDuration := TruncateDuration
	tmpTimeout := Timeout

	local, err := net.GenPeer()
	require.NoError(t, err)

	t.Cleanup(func() {
		ctrl.Finish()

		TruncateDuration = tmpTruncateDuration
		Timeout = tmpTimeout

		wrapdiscovery = wrap.Discovery{}
		wraptime = wrap.Time{}
	})

	return ctrl, local
}

func TestAdvertiser_Advertise(t *testing.T) {
	ctrl, local := setup(t)

	d := mock.NewMockDiscoverer(ctrl)
	wrapdiscovery = d

	chanID := 333

	a := NewAdvertiser(local)

	var wg sync.WaitGroup
	wg.Add(1)

	d.EXPECT().
		NewMdnsService(gomock.Any(), a.did.DiscoveryID(chanID), gomock.Any()).
		DoAndReturn(func(peerhost host.Host, serviceTag string, notifee mdns.Notifee) (mdns.Service, error) {
			wg.Done()
			return DummyMDNSService{}, nil
		}).
		Times(1)

	go func() {
		err := a.Advertise(chanID)
		assert.NoError(t, err)
		wg.Done()
	}()
	wg.Wait()
	wg.Add(1)

	a.Shutdown()
	wg.Wait()
}

func TestAdvertiser_Advertise_multipleTimes(t *testing.T) {
	ctrl, local := setup(t)

	Timeout = time.Millisecond

	d := mock.NewMockDiscoverer(ctrl)
	wrapdiscovery = d

	chanID := 333

	a := NewAdvertiser(local)

	var wg sync.WaitGroup
	wg.Add(5)

	d.EXPECT().
		NewMdnsService(gomock.Any(), a.did.DiscoveryID(chanID), gomock.Any()).
		DoAndReturn(func(peerhost host.Host, serviceTag string, notifee mdns.Notifee) (mdns.Service, error) {
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

func (mdns DummyMDNSService) Start() error {
	return nil
}

func (mdns DummyMDNSService) Close() error {
	return nil
}
