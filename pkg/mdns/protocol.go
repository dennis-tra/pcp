package mdns

import (
	"time"

	"github.com/dennis-tra/pcp/pkg/service"

	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

// protocol encapsulates the logic for discovering peers
// via multicast DNS in the local network.
type protocol struct {
	*pcpnode.Node
	*service.Service
	interval time.Duration
}

func newProtocol(node *pcpnode.Node) *protocol {
	return &protocol{
		Node:     node,
		interval: time.Second,
		Service:  service.New(),
	}
}
