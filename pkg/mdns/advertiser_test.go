package mdns

import (
	"context"
	"sync"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dennis-tra/pcp/internal/mock"
	"github.com/dennis-tra/pcp/internal/wrap"
)

func setup(t *testing.T) (*gomock.Controller, host.Host, func(t *testing.T)) {
	wrapdiscovery = wrap.Discovery{}
	wraptime = wrap.Time{}

	ctrl := gomock.NewController(t)

	net := mocknet.New(context.Background())

	tmpTruncateDuration := TruncateDuration
	tmpTimeout := Timeout

	local, err := net.GenPeer()
	require.NoError(t, err)

	return ctrl, local, func(t *testing.T) {
		ctrl.Finish()

		TruncateDuration = tmpTruncateDuration
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
		NewMdnsService(a, gomock.Any(), gomock.Any()).
		DoAndReturn(func(peerhost host.Host, serviceTag string, notifee mdns.Notifee) mdns.Service {
			defer wg.Done()
			return DummyMDNSService{}
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

type DummyMDNSService struct{}

func (mdns DummyMDNSService) Close() error {
	return nil
}

func (mdns DummyMDNSService) Start() error {
	return nil
}
