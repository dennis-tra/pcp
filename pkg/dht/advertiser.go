package dht

import (
	"context"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"time"

	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

type Advertiser struct {
	*protocol
	shutdown chan struct{}
	done     chan struct{}
}

func NewAdvertiser(node *pcpnode.Node) *Advertiser {
	return &Advertiser{
		protocol: newProtocol(node),
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (a *Advertiser) Advertise(ctx context.Context, code string) error {

	if err := a.Bootstrap(ctx); err != nil {
		return err
	}

	h, err := mh.Sum([]byte(code), mh.SHA2_256, -1)
	if err != nil {
		return err
	}

	a.shutdown = make(chan struct{})
	a.done = make(chan struct{})
	defer close(a.done)

	for {
		queryDone := make(chan struct{})
		cancelCtx, cancel := context.WithCancel(ctx)

		go func() {
			// this context requires a timeout; it determines how long the DHT looks for
			// closest peers to the key/CID before it goes on to provide the record to them.
			// Not setting a timeout here will make the DHT wander forever.
			pctx, cancel := context.WithTimeout(cancelCtx, 60*time.Second)
			defer cancel()

			err = a.DHT.Provide(pctx, cid.NewCidV1(cid.Raw, h), true)
			if err != nil && err != context.Canceled {
				log.Warningln("Error providing", err)
			}

			close(queryDone)
		}()

		select {
		case <-queryDone:
			continue
		case <-ctx.Done():
			cancel()
			return nil
		case <-a.shutdown:
			cancel()
			return nil
		}
	}
}

func (a *Advertiser) Stop() error {
	if a.shutdown == nil {
		return nil
	}

	close(a.shutdown)
	<-a.done

	a.shutdown = nil
	a.done = nil

	return nil
}
