package dht

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/dennis-tra/pcp/internal/log"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

type Advertiser struct {
	*protocol
}

func NewAdvertiser(node *pcpnode.Node) *Advertiser {
	return &Advertiser{protocol: newProtocol(node)}
}

func (a *Advertiser) Advertise(code string) error {
	if err := a.ServiceStarted(); err != nil {
		return err
	}
	defer a.ServiceStopped()

	if err := a.Bootstrap(); err != nil {
		return err
	}

	contentID, err := strToCid(code)
	if err != nil {
		return err
	}

	for {
		// Only advertise in the DHT if we have a public addr.
		if !a.HasPublicAddr() {
			select {
			case <-a.SigShutdown():
				return nil
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		err = a.provide(contentID)
		if err == context.Canceled {
			break
		} else if err != nil && err != context.DeadlineExceeded {
			log.Warningf("Error providing %s: %s\n", contentID, err)
		}
	}

	return nil
}

func (a *Advertiser) Shutdown() {
	a.Service.Shutdown()
}

// this context requires a timeout; it determines how long the DHT looks for
// closest peers to the key/CID before it goes on to provide the record to them.
// Not setting a timeout here will make the DHT wander forever.
func (a *Advertiser) provide(contentID cid.Cid) error {
	ctx, cancel := context.WithTimeout(a.ServiceContext(), time.Minute)
	defer cancel()
	return a.DHT.Provide(ctx, contentID, true)
}
