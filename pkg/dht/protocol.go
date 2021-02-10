package dht

import (
	"context"
	"github.com/dennis-tra/pcp/internal/log"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"sync"
)

// protocol encapsulates the logic for discovering peers
// through providing it in the IPFS DHT.
type protocol struct {
	*pcpnode.Node
	init sync.Once
}

func newProtocol(node *pcpnode.Node) *protocol {
	return &protocol{Node: node}
}

// Bootstrap connects to a set of bootstrap nodes to connect
// to the DHT. This function is a no-op after it is called.
//
// TODO: Make bootstrap nodes configurable
// TODO: Make it possible to bootstrap from local IPFS node
// TODO: Exit early after we have established min 3 connections
func (p *protocol) Bootstrap(ctx context.Context) error {

	var err error
	p.init.Do(func() {
		log.Info("Bootstrap DHT...")

		bootstrapPeers := []string{
			"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
			"/ip4/159.69.43.228/tcp/4001/p2p/QmSKVUFAyCddg2wDUdZVCfvqG5YCwwJTWY1HRmorebXcKG",
			"/ip4/103.25.23.251/tcp/4001/p2p/Qmdo6uKt4u23wegnU8yhR1PKXP3RAqwiTvqymJY1PmccsQ",
		}

		var wg sync.WaitGroup
		for _, bp := range bootstrapPeers {
			ma, err := multiaddr.NewMultiaddr(bp)
			if err != nil {
				return
			}

			peerInfo, err := peer.AddrInfoFromP2pAddr(ma)
			if err != nil {
				return
			}

			wg.Add(1)
			go func(pi peer.AddrInfo) {
				defer wg.Done()
				if err := p.Connect(ctx, pi); err != nil {
					log.Infoln("Error connecting to bootstrap peer:", err)
				}
			}(*peerInfo)
		}

		wg.Wait()

		log.Infoln("Bootstrap DHT Done!")
	})

	return err
}
