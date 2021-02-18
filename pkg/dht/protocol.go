package dht

import (
	"sync"

	"github.com/dennis-tra/pcp/pkg/service"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	mh "github.com/multiformats/go-multihash"

	"github.com/dennis-tra/pcp/internal/log"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

// protocol encapsulates the logic for discovering peers
// through providing it in the IPFS DHT.
type protocol struct {
	*pcpnode.Node
	*service.Service
	init sync.Once
}

func newProtocol(node *pcpnode.Node) *protocol {
	return &protocol{Node: node, Service: service.New()}
}

// Bootstrap connects to a set of bootstrap nodes to connect
// to the DHT. This function is a no-op after it is called.
//
// TODO: Make bootstrap nodes configurable
// TODO: Make it possible to bootstrap from local IPFS node
// TODO: Exit early after we have established min 3 connections
func (p *protocol) Bootstrap() error {
	var err error
	p.init.Do(func() {

		var wg sync.WaitGroup
		for _, bp := range dht.GetDefaultBootstrapPeerAddrInfos() {
			wg.Add(1)
			go func(pi peer.AddrInfo) {
				defer wg.Done()
				if err := p.Connect(p.ServiceContext(), pi); err != nil {
					log.Infoln("Error connecting to bootstrap peer:", err)
				}
			}(bp)
		}

		wg.Wait()
	})

	return err
}

func strToCid(str string) (cid.Cid, error) {
	h, err := mh.Sum([]byte(str), mh.SHA2_256, -1)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(cid.Raw, h), nil
}
