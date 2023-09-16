package mdns

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/dennis-tra/pcp/internal/mock"
	"github.com/dennis-tra/pcp/pkg/discovery"
	pcptest "github.com/dennis-tra/pcp/test"
)

func TestNew(t *testing.T) {
	chanID := 2
	sender := pcptest.VoidSender{}
	h := mock.NewMockHost(gomock.NewController(t))

	model := New(h, sender, chanID)

	assert.Equal(t, sender, model.sender)
	assert.Equal(t, h, model.host)
	assert.Equal(t, StateIdle, model.State)
	assert.Equal(t, chanID, model.chanID)
	assert.NotNil(t, model.services)
	assert.NotNil(t, model.spinner)
	assert.Nil(t, model.Err)
	assert.NotNil(t, model.newMdnsService)
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{state: StateIdle, want: "StateIdle"},
		{state: StateStarted, want: "StateStarted"},
		{state: StateError, want: "StateError"},
		{state: StateStopped, want: "StateStopped"},
		{state: State("random"), want: "StateUnknown"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s = %s", tt.state, tt.want), func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.state.String(), "String()")
		})
	}
}

func TestModel_Init(t *testing.T) {
	chanID := 2
	sender := pcptest.VoidSender{}
	h := mock.NewMockHost(gomock.NewController(t))

	model := New(h, sender, chanID)

	assert.NotNil(t, model.Init())
}

func TestModel_newService(t *testing.T) {
	chanID := 2
	sender := pcptest.VoidSender{}
	ctrl := gomock.NewController(t)
	h := mock.NewMockHost(ctrl)

	model := New(h, sender, chanID)
	offset := 5 * time.Second

	mdnsService := mock.NewMockMDNSService(ctrl)
	model.newMdnsService = func(host host.Host, s string, notifee mdns.Notifee) mdns.Service {
		assert.Equal(t, h, host)
		assert.Equal(t, s, discovery.NewID(offset).DiscoveryID(chanID))
		assert.Equal(t, model, notifee)
		return mdnsService
	}
	mdnsService.EXPECT().Start().Times(1)
	svc, err := model.newService(offset)
	require.NoError(t, err)
	assert.Equal(t, mdnsService, svc)
}

func TestModel_newService_error(t *testing.T) {
	ctrl := gomock.NewController(t)
	model := New(nil, nil, 0)

	offset := 5 * time.Second

	mdnsService := mock.NewMockMDNSService(ctrl)
	model.newMdnsService = func(host host.Host, s string, notifee mdns.Notifee) mdns.Service {
		return mdnsService
	}

	mdnsService.EXPECT().Start().Times(1).Return(fmt.Errorf("some error"))
	svc, err := model.newService(offset)
	assert.Error(t, err)
	assert.Nil(t, svc)
}

func TestModel_Start(t *testing.T) {
	chanID := 2
	sender := pcptest.VoidSender{}
	ctrl := gomock.NewController(t)
	h := mock.NewMockHost(ctrl)

	model := New(h, sender, chanID)

	offset1 := 5 * time.Second
	offset2 := 10 * time.Second

	mdnsService1 := mock.NewMockMDNSService(ctrl)
	mdnsService2 := mock.NewMockMDNSService(ctrl)

	callCount := 0
	model.newMdnsService = func(host host.Host, s string, notifee mdns.Notifee) mdns.Service {
		callCount += 1
		if callCount == 1 {
			assert.Equal(t, s, discovery.NewID(offset1).DiscoveryID(chanID))
			return mdnsService1
		} else {
			assert.Equal(t, s, discovery.NewID(offset2).DiscoveryID(chanID))
			return mdnsService2
		}
	}
	mdnsService1.EXPECT().Start().Times(1)
	mdnsService2.EXPECT().Start().Times(1)

	model, cmd := model.Start(offset1, offset2)

	assert.Equal(t, StateStarted, model.State)
	assert.Len(t, model.services, 2)
	assert.Nil(t, model.Err)

	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)
	assert.Len(t, batch, 2)
}

func TestModel_Start_error(t *testing.T) {
	chanID := 2
	sender := pcptest.VoidSender{}
	ctrl := gomock.NewController(t)
	h := mock.NewMockHost(ctrl)

	model := New(h, sender, chanID)

	offset1 := 5 * time.Second
	offset2 := 10 * time.Second

	mdnsService1 := mock.NewMockMDNSService(ctrl)
	mdnsService2 := mock.NewMockMDNSService(ctrl)

	callCount := 0
	model.newMdnsService = func(host host.Host, s string, notifee mdns.Notifee) mdns.Service {
		callCount += 1
		if callCount == 1 {
			assert.Equal(t, s, discovery.NewID(offset1).DiscoveryID(chanID))
			return mdnsService1
		} else {
			assert.Equal(t, s, discovery.NewID(offset2).DiscoveryID(chanID))
			return mdnsService2
		}
	}
	mdnsService1.EXPECT().Start().Times(1)
	mdnsService1.EXPECT().Close().Times(1) // Started service should be closed

	mdnsService2.EXPECT().Start().Times(1).Return(fmt.Errorf("some error"))

	model, cmd := model.Start(offset1, offset2)

	assert.Equal(t, StateError, model.State)
	assert.Error(t, model.Err)
	assert.Len(t, model.services, 0)
	assert.Nil(t, cmd)
}
