package dht

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dennis-tra/pcp/internal/mock"
	"github.com/dennis-tra/pcp/pkg/service"
)

func TestAdvertiser_Advertise(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	dht := mock.NewMockIpfsDHT(ctrl)
	a := NewAdvertiser(local, dht)

	var wg sync.WaitGroup
	wg.Add(5)

	dht.EXPECT().
		Provide(gomock.Any(), gomock.Any(), true).
		DoAndReturn(func(ctx context.Context, c cid.Cid, brdcst bool) (err error) {
			wg.Done()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
				return nil
			}
		}).AnyTimes()

	go func() {
		wg.Wait()
		a.Shutdown()
	}()

	err := a.Advertise(333)
	assert.NoError(t, err)
}

func TestAdvertiser_Advertise_deadlineExceeded(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	provideTimeout = 10 * time.Millisecond

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	dht := mock.NewMockIpfsDHT(ctrl)
	a := NewAdvertiser(local, dht)

	var wg sync.WaitGroup
	wg.Add(5)

	dht.EXPECT().
		Provide(gomock.Any(), gomock.Any(), true).
		DoAndReturn(func(ctx context.Context, c cid.Cid, brdcst bool) (err error) {
			wg.Done()
			<-ctx.Done()
			return ctx.Err()
		}).Times(5)

	go func() {
		wg.Wait()
		a.Shutdown()
	}()

	err := a.Advertise(333)
	assert.NoError(t, err)
}

func TestAdvertiser_Advertise_provideAfterPublicAddr(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	pubAddrInter = 10 * time.Millisecond

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	dht := mock.NewMockIpfsDHT(ctrl)
	manet := mock.NewMockManeter(ctrl)

	a := NewAdvertiser(local, dht)

	gomock.InOrder(
		manet.EXPECT().IsPublicAddr(local.Addrs()[0]).Return(false).Times(1),
		manet.EXPECT().IsPublicAddr(local.Addrs()[0]).Return(false).Times(1),
		manet.EXPECT().IsPublicAddr(local.Addrs()[0]).Return(true).Times(1),
	)
	wrapmanet = manet

	var wg sync.WaitGroup
	wg.Add(1)

	dht.EXPECT().
		Provide(gomock.Any(), gomock.Any(), true).
		DoAndReturn(func(ctx context.Context, c cid.Cid, brdcst bool) (err error) {
			wg.Done()
			<-ctx.Done()
			return ctx.Err()
		}).Times(1)

	go func() {
		wg.Wait()
		a.Shutdown()
	}()

	err := a.Advertise(333)
	assert.NoError(t, err)
}

func TestAdvertiser_Advertise_listensOnShutdownIfNoPublicAddr(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	dht := mock.NewMockIpfsDHT(ctrl)
	manet := mock.NewMockManeter(ctrl)

	var wg sync.WaitGroup
	wg.Add(2)

	manet.EXPECT().
		IsPublicAddr(local.Addrs()[0]).
		DoAndReturn(func(a ma.Multiaddr) bool {
			wg.Done()
			return false
		}).
		AnyTimes()
	wrapmanet = manet

	a := NewAdvertiser(local, dht)
	dht.EXPECT().
		Provide(gomock.Any(), gomock.Any(), true).
		Times(0)

	go func() {
		wg.Wait()
		a.Shutdown()
	}()

	err := a.Advertise(333)
	assert.NoError(t, err)
}

func TestAdvertiser_Advertise_propagatesServiceAlreadyStarted(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	a := NewAdvertiser(local, mock.NewMockIpfsDHT(ctrl))

	err := a.ServiceStarted()
	require.NoError(t, err)

	err = a.Advertise(333)
	assert.Equal(t, service.ErrServiceAlreadyStarted, err)
}

func TestAdvertiser_Advertise_continuesToProvideOnError(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	TruncateDuration = 10 * time.Millisecond

	mockDefaultBootstrapPeers(t, ctrl, net, local)
	dht := mock.NewMockIpfsDHT(ctrl)

	var wg sync.WaitGroup
	wg.Add(5)

	var cids []string

	dht.EXPECT().
		Provide(gomock.Any(), gomock.Any(), true).
		DoAndReturn(func(ctx context.Context, c cid.Cid, brdcst bool) (err error) {
			cids = append(cids, c.String())
			wg.Done()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
				return fmt.Errorf("some error")
			}
		}).Times(5)

	a := NewAdvertiser(local, dht)

	go func() {
		wg.Wait()
		a.Shutdown()
	}()

	err := a.Advertise(333)
	require.NoError(t, err)
}

func TestAdvertiser_Advertise_mutatesDiscoveryIdentifier(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	TruncateDuration = 10 * time.Millisecond

	mockDefaultBootstrapPeers(t, ctrl, net, local)
	dht := mock.NewMockIpfsDHT(ctrl)

	var wg sync.WaitGroup
	wg.Add(2)

	var cids []string

	dht.EXPECT().
		Provide(gomock.Any(), gomock.Any(), true).
		DoAndReturn(func(ctx context.Context, c cid.Cid, brdcst bool) (err error) {
			cids = append(cids, c.String())
			wg.Done()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
				return nil
			}
		}).Times(2)

	a := NewAdvertiser(local, dht)

	go func() {
		wg.Wait()
		a.Shutdown()
	}()

	err := a.Advertise(333)
	require.NoError(t, err)

	assert.NotEqual(t, cids[0], cids[1])
}
