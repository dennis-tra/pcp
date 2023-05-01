package dht

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dennis-tra/pcp/internal/mock"
	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/service"
)

type eventBusHost struct {
	host.Host
	bus *dummyEventBus
}

func newEventBusHost(h host.Host, eventsChan chan interface{}) host.Host {
	return &eventBusHost{
		Host: h,
		bus:  &dummyEventBus{eventsChan},
	}
}

func (h *eventBusHost) EventBus() event.Bus {
	return h.bus
}

type dummyEventBus struct {
	eventsChan chan interface{}
}

var _ event.Bus = (*dummyEventBus)(nil)

func (d *dummyEventBus) Subscribe(eventType interface{}, opts ...event.SubscriptionOpt) (event.Subscription, error) {
	return &dummyEventSubscription{d.eventsChan}, nil
}

func (d *dummyEventBus) Emitter(eventType interface{}, opts ...event.EmitterOpt) (event.Emitter, error) {
	return nil, nil
}

func (d *dummyEventBus) GetAllEventTypes() []reflect.Type {
	return nil
}

type dummyEventSubscription struct {
	c chan interface{}
}

var _ event.Subscription = (*dummyEventSubscription)(nil)

func (sub *dummyEventSubscription) Close() error {
	return nil
}

func (sub *dummyEventSubscription) Out() <-chan interface{} {
	return sub.c
}

func (sub *dummyEventSubscription) Name() string {
	return "dummy event subscription"
}

func TestAdvertiser_Advertise(t *testing.T) {
	ctrl, local, net := setup(t)

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	eventsChan := make(chan interface{})
	local = newEventBusHost(local, eventsChan)

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

	go func() {
		eventsChan <- event.EvtLocalReachabilityChanged{
			Reachability: network.ReachabilityPublic,
		}
	}()

	a.Advertise(333)
}

func TestAdvertiser_Advertise_deadlineExceeded(t *testing.T) {
	ctrl, local, net := setup(t)

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	eventsChan := make(chan interface{})
	local = newEventBusHost(local, eventsChan)

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

	go func() {
		eventsChan <- event.EvtLocalReachabilityChanged{
			Reachability: network.ReachabilityPublic,
		}
	}()

	a.Advertise(333)
}

func TestAdvertiser_Advertise_propagatesServiceAlreadyStarted(t *testing.T) {
	ctrl, local, _ := setup(t)

	eventsChan := make(chan interface{})
	local = newEventBusHost(local, eventsChan)

	a := NewAdvertiser(local, mock.NewMockIpfsDHT(ctrl))

	err := a.ServiceStarted()
	require.NoError(t, err)

	go func() {
		eventsChan <- event.EvtLocalReachabilityChanged{network.ReachabilityPublic}
	}()

	a.Advertise(333)
	assert.Equal(t, service.ErrServiceAlreadyStarted, a.state.Err)
}

func TestAdvertiser_Advertise_continuesToProvideOnError(t *testing.T) {
	ctrl, local, net := setup(t)

	discovery.TruncateDuration = 10 * time.Millisecond

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	eventsChan := make(chan interface{})
	local = newEventBusHost(local, eventsChan)

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

	go func() {
		eventsChan <- event.EvtLocalReachabilityChanged{network.ReachabilityPublic}
	}()

	a.Advertise(333)
}

func TestAdvertiser_Advertise_mutatesDiscoveryIdentifier(t *testing.T) {
	ctrl, local, net := setup(t)

	discovery.TruncateDuration = 10 * time.Millisecond

	mockDefaultBootstrapPeers(t, ctrl, net, local)

	eventsChan := make(chan interface{})
	local = newEventBusHost(local, eventsChan)

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

	go func() {
		eventsChan <- event.EvtLocalReachabilityChanged{network.ReachabilityPublic}
	}()

	a.Advertise(333)

	assert.NotEqual(t, cids[0], cids[1])
}

func TestAdvertiser_analyzeNetwork(t *testing.T) {
	ctrl, local, _ := setup(t)

	eventsChan := make(chan interface{})
	eventBustHost := newEventBusHost(local, eventsChan)

	dht := mock.NewMockIpfsDHT(ctrl)
	a := NewAdvertiser(eventBustHost, dht)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := a.analyzeNetwork()
		assert.Error(t, err) // we expect an error because we closed the channel
		wg.Done()
	}()

	// test reachability
	assert.Equal(t, a.state.Reachability, network.ReachabilityUnknown)
	eventsChan <- event.EvtLocalReachabilityChanged{
		Reachability: network.ReachabilityPrivate,
	}
	eventsChan <- struct{}{} // makes sure the event was consumed
	assert.Equal(t, a.state.Reachability.String(), network.ReachabilityPrivate.String())
	eventsChan <- event.EvtLocalReachabilityChanged{
		Reachability: network.ReachabilityUnknown,
	}
	eventsChan <- struct{}{} // makes sure the event was consumed
	assert.Equal(t, a.state.Reachability.String(), network.ReachabilityUnknown.String())

	// test NAT type UDP
	assert.Equal(t, a.state.NATTypeUDP.String(), network.NATDeviceTypeUnknown.String())
	eventsChan <- event.EvtNATDeviceTypeChanged{
		NatDeviceType:     network.NATDeviceTypeSymmetric,
		TransportProtocol: network.NATTransportUDP,
	}
	eventsChan <- struct{}{} // makes sure the event was consumed
	assert.Equal(t, a.state.NATTypeUDP.String(), network.NATDeviceTypeSymmetric.String())

	// test NAT type TCP
	assert.Equal(t, a.state.NATTypeTCP.String(), network.NATDeviceTypeUnknown.String())
	eventsChan <- event.EvtNATDeviceTypeChanged{
		NatDeviceType:     network.NATDeviceTypeSymmetric,
		TransportProtocol: network.NATTransportTCP,
	}
	eventsChan <- struct{}{} // makes sure the event was consumed
	assert.Equal(t, a.state.NATTypeTCP.String(), network.NATDeviceTypeSymmetric.String())

	// test address update
	assert.Len(t, a.state.PrivateAddrs, 0)
	assert.Len(t, a.state.PublicAddrs, 1) // from mock net
	assert.Len(t, a.state.RelayAddrs, 0)
	privateAddr, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/5000")
	require.NoError(t, err)
	publicAddr, err := ma.NewMultiaddr("/ip4/200.200.200.200/udp/5000")
	require.NoError(t, err)
	relayAddr, err := ma.NewMultiaddr("/ip4/200.200.200.200/udp/5000/p2p-circuit")
	require.NoError(t, err)

	eventsChan <- event.EvtLocalAddressesUpdated{
		Current: []event.UpdatedAddress{
			{Address: privateAddr},
			{Address: publicAddr},
			{Address: relayAddr},
		},
	}
	eventsChan <- struct{}{} // makes sure the event was consumed
	assert.Equal(t, privateAddr, a.state.PrivateAddrs[0])
	assert.Equal(t, publicAddr, a.state.PublicAddrs[0])
	assert.Equal(t, relayAddr, a.state.RelayAddrs[0])

	close(eventsChan)

	wg.Wait()
}

func TestAdvertiser_analyzeNetwork_terminate_symmetric_nat(t *testing.T) {
	ctrl, local, _ := setup(t)

	eventsChan := make(chan interface{})
	defer close(eventsChan)

	eventBustHost := newEventBusHost(local, eventsChan)

	dht := mock.NewMockIpfsDHT(ctrl)
	a := NewAdvertiser(eventBustHost, dht)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		eventsChan <- event.EvtLocalReachabilityChanged{
			Reachability: network.ReachabilityPrivate,
		}
		eventsChan <- event.EvtNATDeviceTypeChanged{
			NatDeviceType:     network.NATDeviceTypeSymmetric,
			TransportProtocol: network.NATTransportUDP,
		}
		eventsChan <- event.EvtNATDeviceTypeChanged{
			NatDeviceType:     network.NATDeviceTypeSymmetric,
			TransportProtocol: network.NATTransportTCP,
		}
		wg.Done()
	}()

	err := a.analyzeNetwork()
	assert.Error(t, err)
	wg.Wait()
}

func TestAdvertiser_analyzeNetwork_terminate_public(t *testing.T) {
	ctrl, local, _ := setup(t)

	eventsChan := make(chan interface{})
	defer close(eventsChan)

	eventBustHost := newEventBusHost(local, eventsChan)

	dht := mock.NewMockIpfsDHT(ctrl)
	a := NewAdvertiser(eventBustHost, dht)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// already has public address from mocknet
		eventsChan <- event.EvtLocalReachabilityChanged{
			Reachability: network.ReachabilityPublic,
		}
		wg.Done()
	}()

	err := a.analyzeNetwork()
	assert.NoError(t, err)
	wg.Wait()
}

func TestAdvertiser_analyzeNetwork_terminate_private_cone(t *testing.T) {
	ctrl, local, _ := setup(t)

	eventsChan := make(chan interface{})
	defer close(eventsChan)

	eventBustHost := newEventBusHost(local, eventsChan)

	dht := mock.NewMockIpfsDHT(ctrl)
	a := NewAdvertiser(eventBustHost, dht)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// already has public address from mocknet
		eventsChan <- event.EvtLocalReachabilityChanged{
			Reachability: network.ReachabilityPrivate,
		}
		eventsChan <- event.EvtNATDeviceTypeChanged{
			NatDeviceType:     network.NATDeviceTypeCone,
			TransportProtocol: network.NATTransportUDP,
		}
		eventsChan <- event.EvtNATDeviceTypeChanged{
			NatDeviceType:     network.NATDeviceTypeCone,
			TransportProtocol: network.NATTransportUDP,
		}
		relayAddr, err := ma.NewMultiaddr("/ip4/200.200.200.200/udp/5000/p2p-circuit")
		require.NoError(t, err)

		eventsChan <- event.EvtLocalAddressesUpdated{
			Current: []event.UpdatedAddress{
				{Address: relayAddr},
			},
		}
		wg.Done()
	}()

	err := a.analyzeNetwork()
	assert.NoError(t, err)
	wg.Wait()
}

func TestAdvertiser_analyzeNetwork_terminate_service_stop(t *testing.T) {
	ctrl, local, _ := setup(t)

	dht := mock.NewMockIpfsDHT(ctrl)
	a := NewAdvertiser(local, dht)

	err := a.ServiceStarted()
	require.NoError(t, err)

	go a.Shutdown()

	err = a.analyzeNetwork()
	assert.Error(t, err)
}
