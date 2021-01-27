package node

import (
	"context"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func mockNode(t *testing.T) *Node {
	ctx := context.Background()
	net := mocknet.New(ctx)
	h, err := net.GenPeer()
	require.NoError(t, err)

	return &Node{Host: h}
}

func mockPushNode(t *testing.T) *Node {
	node := mockNode(t)
	node.PushProtocol = NewPushProtocol(node)
	return node
}

func TestNewPushProtocol_returnsInitializedStruct(t *testing.T) {
	node := mockNode(t)
	p := NewPushProtocol(node)

	assert.Equal(t, p.node, node)
	assert.NotNil(t, p.respChans)
}

func TestMockNet(t *testing.T) {

}
