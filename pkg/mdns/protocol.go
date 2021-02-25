package mdns

import (
	"fmt"
	"time"

	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	"github.com/dennis-tra/pcp/pkg/service"
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

// DiscoveryIdentifier returns the string, that we use to advertise
// via mDNS and the DHT. See chanID above for more information.
func (p *protocol) DiscoveryIdentifier(chanID int) string {
	return fmt.Sprintf("/pcp/%d/%d", time.Now().Truncate(5*time.Minute).Unix(), chanID)
}
