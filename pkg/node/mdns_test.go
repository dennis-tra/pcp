package node

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dennis-tra/pcp/internal/app"
	"github.com/dennis-tra/pcp/internal/mock"
	"github.com/dennis-tra/pcp/pkg/commons"
	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setup(t *testing.T) *gomock.Controller {
	appDiscovery = app.Discovery{}
	appTime = app.Time{}
	return gomock.NewController(t)
}

func teardown(t *testing.T, ctrl *gomock.Controller) {
	ctrl.Finish()
	appDiscovery = app.Discovery{}
	appTime = app.Time{}
}

func TestNewMdnsProtocol_returnsInitializedMdnsProtocol(t *testing.T) {
	n := mockNode(t)
	m := NewMdnsProtocol(n)
	assert.Equal(t, n, m.node)
	assert.Equal(t, time.Second, m.MdnsInterval)
	assert.NotNil(t, m.Peers)
}

func TestMdnsProtocol_StartStopMdnsService_happyPath(t *testing.T) {
	n := mockNode(t)
	p := NewMdnsProtocol(n)

	ctrl := setup(t)
	defer teardown(t, ctrl)

	ctx := context.Background()

	mdnsServ := mock.NewMockDiscoveryService(ctrl)
	mdnsServ.EXPECT().
		RegisterNotifee(gomock.Eq(p)).
		Times(1)

	m := mock.NewMockDiscoverer(ctrl)
	m.EXPECT().
		NewMdnsService(gomock.Eq(ctx), gomock.Eq(p.node.Host), gomock.Eq(time.Second), gomock.Eq(commons.ServiceTag)).
		Return(mdnsServ, nil).
		Times(1)

	appDiscovery = m

	err := p.StartMdnsService(ctx)
	require.NoError(t, err)
	assert.Equal(t, mdnsServ, p.mdnsServ)

	// Idempotent if already running
	err = p.StartMdnsService(ctx)
	assert.NoError(t, err)

	pi := PeerInfo{timer: appTime.NewTimer(time.Hour)}
	p.Peers.Store("peer-id", pi)

	mdnsServ.EXPECT().
		UnregisterNotifee(gomock.Eq(p)).
		Times(1)

	mdnsServ.EXPECT().
		Close().
		Return(nil).
		Times(1)

	err = p.StopMdnsService()
	assert.NoError(t, err)

	assert.False(t, pi.timer.Stop())
	_, found := p.Peers.Load("peer-id")
	assert.False(t, found)
	assert.Nil(t, p.mdnsServ)

	// Idempotent if already stopped
	err = p.StopMdnsService()
	assert.NoError(t, err)
}

func TestMdnsProtocol_HandlePeerFound_storesPeer(t *testing.T) {
	n := mockNode(t)
	p := NewMdnsProtocol(n)

	pi := peer.AddrInfo{ID: peer.ID("peer-id")}
	p.HandlePeerFound(pi)

	_, found := p.Peers.Load(peer.ID("peer-id"))
	assert.True(t, found)
}

func TestMdnsProtocol_HandlePeerFound_deletesPeerAfterGCTime(t *testing.T) {
	n := mockNode(t)
	p := NewMdnsProtocol(n)

	ctrl := setup(t)
	defer teardown(t, ctrl)

	gcDurationTmp := gcDuration
	gcDuration = 0 * time.Millisecond
	peerId := peer.ID("peer-id")

	var wg sync.WaitGroup
	wg.Add(1)

	mTime := mock.NewMockTimer(ctrl)
	mTime.EXPECT().
		AfterFunc(gomock.Eq(gcDuration), gomock.Any()).
		Times(1).
		DoAndReturn(func(d time.Duration, f func()) *time.Timer {
			return time.AfterFunc(d, func() {
				f()
				wg.Done()
			})
		})

	appTime = mTime

	pi := peer.AddrInfo{ID: peerId}
	p.HandlePeerFound(pi)

	wg.Wait()

	_, found := p.Peers.Load(peerId)
	assert.False(t, found)

	gcDuration = gcDurationTmp
}

func TestMdnsProtocol_HandlePeerFound_deletesPeerAfterGCTimeWithIntermediateResets(t *testing.T) {
	n := mockNode(t)
	p := NewMdnsProtocol(n)

	ctrl := setup(t)
	defer teardown(t, ctrl)

	gcDurationTmp := gcDuration
	gcDuration = 100 * time.Millisecond
	peerId := peer.ID("peer-id")

	mTime := mock.NewMockTimer(ctrl)
	mTime.EXPECT().
		AfterFunc(gomock.Eq(gcDuration), gomock.Any()).
		Times(1).
		DoAndReturn(time.AfterFunc)
	appTime = mTime

	pi := peer.AddrInfo{ID: peerId}
	p.HandlePeerFound(pi)
	time.Sleep(50 * time.Millisecond)
	p.HandlePeerFound(pi)
	time.Sleep(75 * time.Millisecond)
	_, found := p.Peers.Load(peerId)
	assert.True(t, found)
	time.Sleep(50 * time.Millisecond)
	_, found = p.Peers.Load(peerId)
	assert.False(t, found)

	gcDuration = gcDurationTmp
}

func TestMdnsProtocol_PeerList_returnsListOfPeers(t *testing.T) {
	n := mockNode(t)
	p := NewMdnsProtocol(n)

	p1 := peer.AddrInfo{ID: peer.ID("peer-id-1")}
	p2 := peer.AddrInfo{ID: peer.ID("peer-id-2")}
	p3 := peer.AddrInfo{ID: peer.ID("peer-id-3")}

	p.HandlePeerFound(p1)
	p.HandlePeerFound(p3)
	p.HandlePeerFound(p2)

	list := p.PeersList()

	assert.Equal(t, p1, list[0])
	assert.Equal(t, p2, list[1])
	assert.Equal(t, p3, list[2])
}

func ExampleMdnsProtocol_PrintPeers_doesNotError() {

	ctx := context.Background()
	net := mocknet.New(ctx)
	h, _ := net.GenPeer()

	n := &Node{Host: h}
	p := NewMdnsProtocol(n)

	id1, _ := peer.Decode("QmUfTE3fCY8DvSAG5XGkmVvuqyihxujH52mbm8RCFJGMwD")
	id2, _ := peer.Decode("12D3KooWFDN3sDfvn9dPvFZG4P1hMg21PTmUt56zCtX7VWRZkLV8")
	id3, _ := peer.Decode("QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ")

	p1 := peer.AddrInfo{ID: id1}
	p2 := peer.AddrInfo{ID: id2}
	p3 := peer.AddrInfo{ID: id3}

	p.HandlePeerFound(p1)
	p.HandlePeerFound(p3)
	p.HandlePeerFound(p2)

	p.PrintPeers(p.PeersList())

	// Output:
	// [0] 12D3KooWFDN3sDfvn9dPvFZG4P1hMg21PTmUt56zCtX7VWRZkLV8
	// [1] QmUfTE3fCY8DvSAG5XGkmVvuqyihxujH52mbm8RCFJGMwD
	// [2] QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ
}
