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
}

func NewAdvertiser(node *pcpnode.Node) *Advertiser {
	return &Advertiser{newProtocol(node)}
}

func (a *Advertiser) Advertise(ctx context.Context, code string) error {

	if err := a.Bootstrap(ctx); err != nil {
		return err
	}

	h, err := mh.Sum([]byte(code), mh.SHA2_256, -1)
	if err != nil {
		return err
	}

	// this context requires a timeout; it determines how long the DHT looks for
	// closest peers to the key/CID before it goes on to provide the record to them.
	// Not setting a timeout here will make the DHT wander forever.
	pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	log.Debugln("Advertise: ", cid.NewCidV1(cid.Raw, h).String())
	err = a.DHT.Provide(pctx, cid.NewCidV1(cid.Raw, h), true)
	log.Debugln("Advertise Done!")
	if err != nil {
		return err
	}

	return nil
}

func (a *Advertiser) Stop() error {
	return nil
}
