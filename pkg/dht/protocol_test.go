package dht

import (
	"strconv"
	"strings"
	"sync"
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
)

func setup(t *testing.T) (*gomock.Controller, host.Host, mocknet.Mocknet, func(t *testing.T)) {
	wrapDHT = wrap.DHT{}
	wrapmanet = wrap.Manet{}
	wraptime = wrap.Time{}
	bootstrap = map[peer.ID]*sync.Once{}

	ctrl := gomock.NewController(t)

	net := mocknet.New()

	tmpTruncateDuration := TruncateDuration
	tmpPubAddrInter := pubAddrInter
	tmpProvideTimeout := provideTimeout

	local, err := net.GenPeer()
	require.NoError(t, err)

	return ctrl, local, net, func(t *testing.T) {
		ctrl.Finish()

		TruncateDuration = tmpTruncateDuration
		pubAddrInter = tmpPubAddrInter
		provideTimeout = tmpProvideTimeout

		wrapDHT = wrap.DHT{}
		wrapmanet = wrap.Manet{}
		wraptime = wrap.Time{}
	}
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
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	peers := genPeers(t, net, local, ConnThreshold)
	mockGetDefaultBootstrapPeerAddrInfos(ctrl, peers)

	err := newProtocol(local, nil).Bootstrap()
	require.NoError(t, err)

	// Check if they are connected.
	assert.Len(t, net.Net(local.ID()).Peers(), ConnThreshold)
}

func TestTimeCriticalProtocol_Bootstrap_connectsBootstrapPeersInParallel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping time critical test") // They are flaky on GitHub actions
	}

	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	peers := genPeers(t, net, local, 100)
	mockGetDefaultBootstrapPeerAddrInfos(ctrl, peers)

	latency := 50 * time.Millisecond
	for _, p := range peers {
		links := net.LinksBetweenPeers(local.ID(), p.ID)
		for _, l := range links {
			l.SetOptions(mocknet.LinkOptions{
				Latency: latency,
			})
		}
	}

	start := time.Now()
	err := newProtocol(local, nil).Bootstrap()
	end := time.Now()

	require.NoError(t, err)

	assert.InDelta(t, latency, end.Sub(start), float64(latency)/2)

	// Check if they are connected.
	assert.Len(t, net.Net(local.ID()).Peers(), 100)
}

func TestProtocol_Bootstrap_errorOnNoConfiguredBootstrapPeers(t *testing.T) {
	ctrl, local, _, teardown := setup(t)
	defer teardown(t)

	mockGetDefaultBootstrapPeerAddrInfos(ctrl, []peer.AddrInfo{})

	err := newProtocol(local, nil).Bootstrap()
	assert.Error(t, err)
}

func TestProtocol_Bootstrap_cantConnectToOneLessThanThreshold(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	peers := genPeers(t, net, local, ConnThreshold)
	mockGetDefaultBootstrapPeerAddrInfos(ctrl, peers)

	err := net.UnlinkPeers(local.ID(), peers[0].ID)
	require.NoError(t, err)

	err = newProtocol(local, nil).Bootstrap()
	assert.Error(t, err)

	errs, ok := err.(ErrConnThresholdNotReached)
	assert.True(t, ok)
	assert.Contains(t, errs.BootstrapErrs[0].Error(), local.ID().String())
	assert.Contains(t, errs.BootstrapErrs[0].Error(), peers[0].ID.String())
}

func TestProtocol_Bootstrap_cantConnectToMultipleLessThanThreshold(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	peers := genPeers(t, net, local, ConnThreshold)
	mockGetDefaultBootstrapPeerAddrInfos(ctrl, peers)

	err := net.UnlinkPeers(local.ID(), peers[0].ID)
	require.NoError(t, err)

	err = net.UnlinkPeers(local.ID(), peers[1].ID)
	require.NoError(t, err)

	err = newProtocol(local, nil).Bootstrap()
	assert.Error(t, err)

	errs, ok := err.(ErrConnThresholdNotReached)
	assert.True(t, ok)
	assert.Contains(t, errs.BootstrapErrs[0].Error(), local.ID().String())
	errStr := strings.Join([]string{errs.BootstrapErrs[0].Error(), errs.BootstrapErrs[1].Error()}, " ")
	assert.Contains(t, errStr, peers[0].ID.String())
	assert.Contains(t, errStr, peers[1].ID.String())
}

func TestProtocol_Bootstrap_cantConnectButGreaterThanThreshold(t *testing.T) {
	ctrl, local, net, teardown := setup(t)
	defer teardown(t)

	peers := genPeers(t, net, local, ConnThreshold+1)
	mockGetDefaultBootstrapPeerAddrInfos(ctrl, peers)

	err := net.UnlinkPeers(local.ID(), peers[0].ID)
	require.NoError(t, err)

	err = newProtocol(local, nil).Bootstrap()
	assert.NoError(t, err)
}

func TestProtocol_DiscoveryIdentifier_returnsCorrect(t *testing.T) {
	ctrl, local, _, teardown := setup(t)
	defer teardown(t)

	now := time.Now()

	m := mock.NewMockTimer(ctrl)
	m.EXPECT().Now().Return(now)
	wraptime = m

	p := newProtocol(local, nil)
	id := p.DiscoveryID(333)

	unixNow := now.Truncate(TruncateDuration).UnixNano()
	assert.Equal(t, "/pcp/"+strconv.Itoa(int(unixNow))+"/333", id)
}
