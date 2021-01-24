package receive

import (
	"context"
	"fmt"
	"os"

	"github.com/dennis-tra/pcp/pkg/node"
	"github.com/libp2p/go-libp2p"
)

type Node struct {
	*node.Node
}

func InitNode(ctx context.Context, host string, port int64) (*Node, error) {

	hostAddr := fmt.Sprintf("/ip4/%s/tcp/%d", host, port)
	nn, err := node.Init(ctx, libp2p.ListenAddrStrings(hostAddr))
	if err != nil {
		return nil, err
	}

	n := &Node{nn}

	return n, nil
}

func (n *Node) Close() error {
	err := n.Host.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	err = n.StopMdnsService()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	return nil
}
