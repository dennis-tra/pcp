package dht

import (
	"context"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/discovery"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
	"github.com/ipfs/go-cid"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	mh "github.com/multiformats/go-multihash"
)

type Discoverer struct {
	*protocol
	shutdown chan struct{}
	done     chan struct{}
}

func NewDiscoverer(node *pcpnode.Node) *Discoverer {
	return &Discoverer{protocol: newProtocol(node)}
}

func (d *Discoverer) Discover(ctx context.Context, code string, handler discovery.PeerHandler) error {

	if err := d.Bootstrap(ctx); err != nil {
		return err
	}

	h, err := mh.Sum([]byte(code), mh.SHA2_256, -1)
	if err != nil {
		return err
	}

	d.shutdown = make(chan struct{})
	d.done = make(chan struct{})
	defer close(d.done)

	for {
		queryDone := make(chan struct{})
		cctx, cancel := context.WithCancel(ctx)
		go func() {
			for pi := range d.DHT.FindProvidersAsync(cctx, cid.NewCidV1(cid.Raw, h), 100) {

				// Filter out addresses that are local - only allow public ones.
				routable := []ma.Multiaddr{}
				for _, addr := range pi.Addrs {
					if manet.IsPublicAddr(addr) {
						routable = append(routable, addr)
					}
				}

				if len(routable) == 0 {
					log.Infoln("Found peer that is not publicly accessible: ", pi.ID)
					continue
				}

				log.Infoln("Found peer: ", pi.ID)
				pi.Addrs = routable
				go handler.HandlePeer(pi)
			}
			close(queryDone)
		}()

		select {
		case <-queryDone:
			cancel()
			continue
		case <-d.shutdown:
		case <-ctx.Done():
		}

		cancel()
		<-queryDone
		return nil
	}
}

func (d *Discoverer) Stop() error {
	if d.shutdown == nil {
		return nil
	}

	close(d.shutdown)
	<-d.done

	d.shutdown = nil
	d.done = nil

	return nil
}
