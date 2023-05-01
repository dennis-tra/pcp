package dht

import (
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dennis-tra/pcp/internal/mock"
	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/dennis-tra/pcp/pkg/discovery"
)

type dummyState struct{}

func (d dummyState) SetStage(stage Stage) {
}

func (d dummyState) SetError(err error) {
}

func setup(t *testing.T) (*gomock.Controller, host.Host, mocknet.Mocknet) {
	wrapDHT = wrap.DHT{}
	wrapmanet = wrap.Manet{}
	wraptime = wrap.Time{}

	ctrl := gomock.NewController(t)

	net := mocknet.New()

	tmpTruncateDuration := discovery.TruncateDuration
	tmpProvideTimeout := provideTimeout
	tmpTickInterval := tickInterval

	provideTimeout = 100 * time.Millisecond
	tickInterval = 10 * time.Millisecond

	local, err := net.GenPeer()
	require.NoError(t, err)

	t.Cleanup(func() {
		ctrl.Finish()

		discovery.TruncateDuration = tmpTruncateDuration
		provideTimeout = tmpProvideTimeout
		tickInterval = tmpTickInterval

		wrapDHT = wrap.DHT{}
		wrapmanet = wrap.Manet{}
		wraptime = wrap.Time{}
	})

	return ctrl, local, net
}

func mockGetDefaultBootstrapPeerAddrInfos(ctrl *gomock.Controller, pis []peer.AddrInfo) {
	mockDHT := mock.NewMockDHTer(ctrl)
	mockDHT.EXPECT().
		GetDefaultBootstrapPeerAddrInfos().
		Return(pis)
	wrapDHT = mockDHT
}

func mockDefaultBootstrapPeers(t *testing.T, ctrl *gomock.Controller, net mocknet.Mocknet, h host.Host) {
	mockDHT := mock.NewMockDHTer(ctrl)
	mockDHT.EXPECT().
		GetDefaultBootstrapPeerAddrInfos().
		Return(genPeers(t, net, h, 3))
	wrapDHT = mockDHT
}

func genPeers(t *testing.T, net mocknet.Mocknet, h host.Host, count int) []peer.AddrInfo {
	var peers []peer.AddrInfo
	for i := 0; i < count; i++ {
		p, err := net.GenPeer()
		require.NoError(t, err)

		_, err = net.LinkPeers(h.ID(), p.ID())
		require.NoError(t, err)

		peers = append(peers, peer.AddrInfo{
			ID:    p.ID(),
			Addrs: p.Addrs(),
		})
	}

	return peers
}

func TestProtocol_Bootstrap_connectsBootstrapPeers(t *testing.T) {
	ctrl, local, net := setup(t)

	peers := genPeers(t, net, local, ConnThreshold)
	mockGetDefaultBootstrapPeerAddrInfos(ctrl, peers)

	err := newProtocol[*dummyState](local, nil).bootstrap()
	require.NoError(t, err)

	// Check if they are connected.
	assert.Len(t, net.Net(local.ID()).Peers(), ConnThreshold)
}

func TestProtocol_Bootstrap_errorOnNoConfiguredBootstrapPeers(t *testing.T) {
	ctrl, local, _ := setup(t)

	mockGetDefaultBootstrapPeerAddrInfos(ctrl, []peer.AddrInfo{})

	err := newProtocol[*dummyState](local, nil).bootstrap()
	assert.Error(t, err)
}

func TestProtocol_Bootstrap_cantConnectToOneLessThanThreshold(t *testing.T) {
	ctrl, local, net := setup(t)

	peers := genPeers(t, net, local, ConnThreshold)
	mockGetDefaultBootstrapPeerAddrInfos(ctrl, peers)

	err := net.UnlinkPeers(local.ID(), peers[0].ID)
	require.NoError(t, err)

	err = newProtocol[*dummyState](local, nil).bootstrap()
	assert.Error(t, err)

	errs, ok := err.(ErrConnThresholdNotReached)
	assert.True(t, ok)
	assert.Contains(t, errs.BootstrapErrs[0].Error(), local.ID().String())
	assert.Contains(t, errs.BootstrapErrs[0].Error(), peers[0].ID.String())
}

func TestProtocol_Bootstrap_cantConnectToMultipleLessThanThreshold(t *testing.T) {
	ctrl, local, net := setup(t)

	peers := genPeers(t, net, local, ConnThreshold)
	mockGetDefaultBootstrapPeerAddrInfos(ctrl, peers)

	err := net.UnlinkPeers(local.ID(), peers[0].ID)
	require.NoError(t, err)

	err = net.UnlinkPeers(local.ID(), peers[1].ID)
	require.NoError(t, err)

	err = newProtocol[*dummyState](local, nil).bootstrap()
	assert.Error(t, err)

	errs, ok := err.(ErrConnThresholdNotReached)
	assert.True(t, ok)
	assert.Contains(t, errs.BootstrapErrs[0].Error(), local.ID().String())
	errStr := strings.Join([]string{errs.BootstrapErrs[0].Error(), errs.BootstrapErrs[1].Error()}, " ")
	assert.Contains(t, errStr, peers[0].ID.String())
	assert.Contains(t, errStr, peers[1].ID.String())
}

func TestProtocol_Bootstrap_cantConnectButGreaterThanThreshold(t *testing.T) {
	ctrl, local, net := setup(t)

	peers := genPeers(t, net, local, ConnThreshold+1)
	mockGetDefaultBootstrapPeerAddrInfos(ctrl, peers)

	err := net.UnlinkPeers(local.ID(), peers[0].ID)
	require.NoError(t, err)

	err = newProtocol[*dummyState](local, nil).bootstrap()
	assert.NoError(t, err)
}
