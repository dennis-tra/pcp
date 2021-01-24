package send

import (
	"context"

	"github.com/dennis-tra/pcp/pkg/node"
)

type Node struct {
	*node.Node
}

func InitNode(ctx context.Context) (*Node, error) {

	n, err := node.Init(ctx)
	if err != nil {
		return nil, err
	}

	return &Node{n}, nil
}
