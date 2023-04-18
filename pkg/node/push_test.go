package node

import (
	"context"
	"testing"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// TestTransferHandler is a mock transfer handler that can be registered for the TransferProtocol.
type TestPushRequestHandler struct {
	handler func(*p2p.PushRequest) (bool, error)
}

func (prh *TestPushRequestHandler) HandlePushRequest(pr *p2p.PushRequest) (bool, error) {
	return prh.handler(pr)
}

func TestPushProtocol_RegisterPushRequestHandler_happyPath(t *testing.T) {
	skipMessageAuth = true

	ctx := context.Background()
	net := mocknet.New()

	node1, _ := setupNode(t, net)
	node2, _ := setupNode(t, net)
	authNodes(t, node1, node2)

	err := net.LinkAll()
	require.NoError(t, err)

	tprh := &TestPushRequestHandler{
		handler: func(pr *p2p.PushRequest) (bool, error) {
			assert.NotEmpty(t, pr.Header.RequestId)
			assert.NotEmpty(t, pr.Header.NodePubKey)
			assert.NotEmpty(t, pr.Header.Signature)
			assert.NotEmpty(t, pr.Header.Timestamp)
			assert.Equal(t, "filename", pr.Name)
			assert.EqualValues(t, 1000, pr.Size)
			assert.True(t, pr.IsDir)
			return true, nil
		},
	}

	node2.RegisterPushRequestHandler(tprh)

	accepted, err := node1.SendPushRequest(ctx, node2.ID(), "filename", 1000, true)
	require.NoError(t, err)

	node2.UnregisterPushRequestHandler()

	assert.True(t, accepted)
}

func TestPushProtocol_RegisterPushRequestHandler_unauthenticated(t *testing.T) {
	skipMessageAuth = true

	ctx := context.Background()
	net := mocknet.New()

	node1, _ := setupNode(t, net)
	node2, _ := setupNode(t, net)

	err := net.LinkAll()
	require.NoError(t, err)

	tprh := &TestPushRequestHandler{
		handler: func(pr *p2p.PushRequest) (bool, error) { return true, nil },
	}

	node2.RegisterPushRequestHandler(tprh)

	accept, err := node1.SendPushRequest(ctx, node2.ID(), "filename", 1000, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream reset")
	assert.False(t, accept)
}
