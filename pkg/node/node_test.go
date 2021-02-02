package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_returnsNode(t *testing.T) {
	ctx := context.Background()

	node, err := Init(ctx)
	require.NoError(t, err)
	assert.NotNil(t, node)
}

//func TestNewHeader(t *testing.T) {
//	node := mockNode(t)
//	hdr, err := node.NewHeader()
//	require.NoError(t, err)
//
//	assert.NotEmpty(t, hdr.RequestId)
//	assert.Equal(t, peer.Encode(node.ID()), hdr.NodeId)
//}
//
//func TestSendProto_returnsErrorIfDialError(t *testing.T) {
//	ctx := context.Background()
//	net, _ := mocknet.WithNPeers(ctx, 2)
//
//	// Don't link hosts
//	//net.LinkAll()
//
//	host1 := net.Hosts()[0]
//	host2 := net.Hosts()[1]
//
//	node1 := &Node{Host: host1}
//	node2 := &Node{Host: host2}
//
//	msg := dummyMessage(t, node1)
//	node1.SendProto(ctx, node2.ID(), msg)
//}
//
//func dummyMessage(t *testing.T, n *Node) *p2p.Header {
//	hdr, err := n.NewHeader()
//	require.NoError(t, err)
//
//	hdr.Payload, err = p2p.NewPushRequest("FILE_NAME", 16, cid.Cid{})
//	require.NoError(t, err)
//
//	return hdr
//}
//
//func TestSendProto(t *testing.T) {
//	ctx := context.Background()
//	net, _ := mocknet.WithNPeers(ctx, 2)
//
//	net.LinkAll()
//
//	host1 := net.Hosts()[0]
//	host2 := net.Hosts()[1]
//
//	node1 := &Node{Host: host1}
//	node2 := &Node{Host: host2}
//
//	hdr := dummyMessage(t, node1)
//
//	node2.SetStreamHandler(ProtocolPushRequest, func(stream network.Stream) {
//		fmt.Fprintln(os.Stderr, "EEEEEE")
//	})
//
//	err = node1.SendProto(context.Background(), node2.ID(), hdr)
//	require.NoError(t, err)
//
//	appTime.Sleep(appTime.Second)
//}
