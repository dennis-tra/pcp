package node

import (
	"context"
	"testing"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"
)

func mockNode(t *testing.T) *Node {
	ctx := context.Background()
	net := mocknet.New(ctx)
	h, err := net.GenPeer()
	require.NoError(t, err)

	return &Node{Host: h}
}
