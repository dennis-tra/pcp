package dht

import (
	"context"
	manet "github.com/multiformats/go-multiaddr/net"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/dennis-tra/pcp/internal/log"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

type Advertiser struct {
	*protocol
	shutdown chan struct{}
	done     chan struct{}
}

func NewAdvertiser(node *pcpnode.Node) *Advertiser {
	return &Advertiser{protocol: newProtocol(node)}
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
		cctx, cancel := context.WithCancel(ctx)
		go func() {

		CheckForPublicAddr:
			hasPublicAddr := false
			for _, addr := range a.Addrs() {
				if manet.IsPublicAddr(addr) {
					hasPublicAddr = true
					break
				}
			}

			if !hasPublicAddr {
				select {
				case <-ctx.Done():
					close(queryDone)
					return
				case <-time.After(500 * time.Millisecond):
					goto CheckForPublicAddr
				}
			}
			log.Infoln("Got a public address -> advertising it!")

			// this context requires a timeout; it determines how long the DHT looks for
			// closest peers to the key/CID before it goes on to provide the record to them.
			// Not setting a timeout here will make the DHT wander forever.
			pctx, cancel := context.WithTimeout(cctx, 60*time.Second)
			defer cancel()

			err = a.DHT.Provide(pctx, cid.NewCidV1(cid.Raw, h), true)
			if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				log.Warningln("Error providing", err)
			}

			log.Infoln("Done advertising it!")

			close(queryDone)
		}()

		select {
		case <-queryDone:
			cancel()
			continue
		case <-ctx.Done():
		case <-a.shutdown:
		}

		cancel()
		<-queryDone
		return nil
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
