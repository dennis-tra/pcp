package node

import (
	"context"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPakeRequestHandler is a mock transfer handler that can be registered for the PakeProtocol.
type TestPakeRequestHandler struct {
	handler func(peerID peer.ID)
}

func (t TestPakeRequestHandler) HandleSuccessfulKeyExchange(peerID peer.ID) {
	t.handler(peerID)
}

func TestPakeProtocol_happyPath(t *testing.T) {
	ctx := context.Background()
	net := mocknet.New()

	node1, _ := setupNode(t, net)
	node2, _ := setupNode(t, net)

	err := net.LinkAll()
	require.NoError(t, err)

	tprh := &TestPakeRequestHandler{
		handler: func(peerID peer.ID) {
			assert.Equal(t, peerID, node2.ID())
		},
	}

	node2.RegisterKeyExchangeHandler(tprh)

	key, err := node1.StartKeyExchange(ctx, node2.ID())
	assert.NoError(t, err)
	assert.NotNil(t, key)

	node2.UnregisterKeyExchangeHandler()
}
