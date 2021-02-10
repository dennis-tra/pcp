package main

import (
	"context"
	"fmt"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"
	mh "github.com/multiformats/go-multihash"
)

var BootstrapPeers = []string{
	"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ", // mars.i.ipfs.io
	"/ip4/159.69.43.228/tcp/4001/p2p/QmSKVUFAyCddg2wDUdZVCfvqG5YCwwJTWY1HRmorebXcKG",  // dtrautwein.eu
	"/ip4/103.25.23.251/tcp/4001/p2p/Qmdo6uKt4u23wegnU8yhR1PKXP3RAqwiTvqymJY1PmccsQ",
}

func main() {
	ctx := context.Background()
	var err error
	var idht *dht.IpfsDHT

	ad := []peer.AddrInfo{}
	for _, p := range BootstrapPeers {
		//Bootstrap object
		ma, err := multiaddr.NewMultiaddr(p)
		if err != nil {
			panic(err)
		}

		info, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			panic(err)
		}
		ad = append(ad, *info)
		//err = h.Connect(context.Background(), *info)
		//if err != nil {
		//	log.Infoln(err)
		//} else {
		//	log.Infoln("Connected", info)
		//}
	}

	h, err := libp2p.New(ctx,
		libp2p.EnableAutoRelay(),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			idht, err = dht.New(ctx, h)
			return idht, err
		}),
	)
	if err != nil {
		panic(err)
	}

	log.Infoln("Before Addrs: ", h.Addrs())
	for _, p := range BootstrapPeers {
		//Bootstrap object
		ma, err := multiaddr.NewMultiaddr(p)
		if err != nil {
			panic(err)
		}

		info, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			panic(err)
		}
		err = h.Connect(context.Background(), *info)
		if err != nil {
			log.Infoln(err)
		} else {
			log.Infoln("Connected", info)
		}
	}
	log.Infoln("After Addrs: ", h.Addrs())
	m, err := mh.Encode([]byte("6b912d16-601d-41ac-949b-8681845aaac3"), mh.SHA2_256)
	if err != nil {
		panic(err)
	}

	log.Infoln("Peer ID: ", h.ID().String())

	c := cid.NewCidV1(cid.Raw, m)
	log.Infoln("Providing ", c.String())
	err = idht.Provide(ctx, c, true)
	if err != nil {
		panic(err)
	}

	fmt.Println(h.ID())

}
