package dht

import (
	"sync"

	"github.com/ipfs/go-cid"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	mh "github.com/multiformats/go-multihash"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"

	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	"github.com/dennis-tra/pcp/pkg/service"
)

// protocol encapsulates the logic for discovering peers
// through providing it in the IPFS DHT.
type protocol struct {
	*pcpnode.Node
	*service.Service
	bootstrap sync.Once
}

func newProtocol(node *pcpnode.Node) *protocol {
	return &protocol{Node: node, Service: service.New()}
}

// Bootstrap connects to a set of bootstrap nodes to connect
// to the DHT. This function is a no-op after it is called once.
func (p *protocol) Bootstrap() error {
	errs, ctx := errgroup.WithContext(p.ServiceContext())
	connCount := atomic.NewInt32(0)
	p.bootstrap.Do(func() {
		for _, bp := range kaddht.GetDefaultBootstrapPeerAddrInfos() {
			errs.Go(func() error {
				err := p.Connect(ctx, bp)
				if err == nil {
					connCount.Inc()
				}
				return err
			})
		}
	})
	err := errs.Wait()
	// If we have established 3 or more connections don't return an error
	if err != nil && connCount.Load() < 3 {
		return err
	}
	return nil
}

func strToCid(str string) (cid.Cid, error) {
	h, err := mh.Sum([]byte(str), mh.SHA2_256, -1)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(cid.Raw, h), nil
}
