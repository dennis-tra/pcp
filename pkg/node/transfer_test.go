package node

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/internal/mock"
	p2p "github.com/dennis-tra/pcp/pkg/pb"

	"github.com/golang/mock/gomock"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransferProtocol_onTransfer_unexpected(t *testing.T) {
	n := mockNode(t)
	p := NewTransferProtocol(n)

	ctrl := gomock.NewController(t)
	streamer := mock.NewMockStreamer(ctrl)
	conner := mock.NewMockConner(ctrl)
	conner.EXPECT().RemotePeer().Return(peer.ID("peer-id"))

	streamer.EXPECT().Conn().Return(conner)
	streamer.EXPECT().Reset().Return(nil)

	buffer := new(bytes.Buffer)
	log.Out = buffer
	p.onTransfer(streamer)

	logOut := strings.TrimSpace(buffer.String())

	assert.Equal(t, logOut, "Received data transfer attempt from unexpected peer")
}

func TestTransferProtocol_onTransfer_unexpectedPlusStreamResetFailed(t *testing.T) {
	n := mockNode(t)
	p := NewTransferProtocol(n)

	ctrl := gomock.NewController(t)
	streamer := mock.NewMockStreamer(ctrl)
	conner := mock.NewMockConner(ctrl)
	conner.EXPECT().RemotePeer().Return(peer.ID("peer-id"))

	streamer.EXPECT().Conn().Return(conner)
	streamer.EXPECT().Reset().Return(errors.New("reset error"))

	buffer := new(bytes.Buffer)
	log.Out = buffer
	p.onTransfer(streamer)

	logOut := strings.TrimSpace(buffer.String())

	assert.Equal(t, logOut, "Received data transfer attempt from unexpected peer\nCouldn't reset stream: reset error")
}

func TestTransferProtocol_Transfer(t *testing.T) {
	authenticateMessages = false
	defer func() { authenticateMessages = true }()
	ctx := context.Background()
	net := mocknet.New(ctx)
	peer1, err := net.GenPeer()
	require.NoError(t, err)
	peer2, err := net.GenPeer()
	require.NoError(t, err)

	err = net.LinkAll()
	require.NoError(t, err)

	p1 := NewTransferProtocol(&Node{Host: peer1})
	p2 := NewTransferProtocol(&Node{Host: peer2})

	pr := &p2p.PushRequest{
		FileSize: 100,
		FileName: "some-file",
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		received, err := p2.Receive(context.Background(), peer1.ID(), pr, new(bytes.Buffer))
		assert.EqualValues(t, received, 100)
		assert.NoError(t, err)
		wg.Done()
	}()
	go func() {
		b := make([]byte, 100)
		acknowledged, err := p1.Transfer(context.Background(), peer2.ID(), bytes.NewBuffer(b), "", 100)
		assert.EqualValues(t, acknowledged, 100)
		assert.NoError(t, err)
		wg.Done()
	}()
	wg.Wait()
}
