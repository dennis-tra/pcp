package wrap

import (
	"context"

	"github.com/ipfs/go-cid"
	stddht "github.com/libp2p/go-libp2p-kad-dht"

	"github.com/libp2p/go-libp2p-core/peer"
)

type DHTer interface {
	GetDefaultBootstrapPeerAddrInfos() []peer.AddrInfo
}

type DHT struct{}

func (d DHT) GetDefaultBootstrapPeerAddrInfos() []peer.AddrInfo {
	return stddht.GetDefaultBootstrapPeerAddrInfos()
}

type IpfsDHT interface {
	Provide(context.Context, cid.Cid, bool) error
	FindProvidersAsync(context.Context, cid.Cid, int) <-chan peer.AddrInfo
}
