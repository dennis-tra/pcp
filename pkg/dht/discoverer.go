package dht

import (
	"context"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

type Discoverer struct {
	*protocol
	shutdown chan struct{}
	done     chan struct{}
}

func NewDiscoverer(node *pcpnode.Node) *Discoverer {
	return &Discoverer{
		protocol: newProtocol(node),
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (d *Discoverer) Discover(ctx context.Context, code string, handler discovery.PeerHandler) error {

	log.Infoln("Discover ", code)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := d.Bootstrap(ctx); err != nil {
		return err
	}

	h, err := mh.Sum([]byte("/pcp/"+code), mh.SHA2_256, -1)
	if err != nil {
		return err
	}

	d.shutdown = make(chan struct{})
	d.done = make(chan struct{})
	defer close(d.done)

	for {

		queryDone := make(chan struct{})
		go func() {
			log.Infoln("Finding providers for: ", cid.NewCidV1(cid.Raw, h))
			for pi := range d.DHT.FindProvidersAsync(ctx, cid.NewCidV1(cid.Raw, h), 100) {
				go handler.HandlePeer(pi)
			}
			log.Infoln("Finding providers for: ", cid.NewCidV1(cid.Raw, h), " Done!")
			close(queryDone)
		}()

		select {
		case <-queryDone:
			continue
		case <-ctx.Done():
			return nil
		case <-d.shutdown:
			return nil
		}
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
