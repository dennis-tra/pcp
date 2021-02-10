package mdns

import (
	"time"

	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

const (
	ServicePrefix = "/pcp"
)

// protocol encapsulates the logic for discovering peers
// via multicast DNS in the local network.
type protocol struct {
	*pcpnode.Node
	interval time.Duration
}
