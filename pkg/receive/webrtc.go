package receive

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	//"github.com/libp2p/go-libp2p"
	star "github.com/dennis-tra/go-libp2p-webrtc-star"
	"github.com/libp2p/go-libp2p"
	peer2 "github.com/libp2p/go-libp2p-core/peer"
	crypto "github.com/libp2p/go-libp2p-crypto"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	peer "github.com/libp2p/go-libp2p-peer"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	yamux "github.com/libp2p/go-libp2p-yamux"
	tcp "github.com/libp2p/go-tcp-transport"
	"github.com/multiformats/go-multiaddr"
	//pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func handleContentID(ctx context.Context, contentID string) error {

	// The user provided a CID or share.ipfs.io link instead of four words.
	fmt.Println("Provided CID", contentID)
	privKey, _, _ := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	identity, _ := peer.IDFromPublicKey(privKey.GetPublic())
	peerstore := pstoremem.NewPeerstore()
	muxer := yamux.DefaultTransport
	transport := star.New(identity, peerstore, muxer)

	starMultiaddr1, _ := multiaddr.NewMultiaddr("/dns4/wrtc-star1.par.dwebops.pub/tcp/443/wss/p2p-webrtc-star")
	starMultiaddr2, _ := multiaddr.NewMultiaddr("/dns4/wrtc-star2.sjc.dwebops.pub/tcp/443/wss/p2p-webrtc-star")

	transport = transport.WithSignalConfiguration(star.SignalConfiguration{
		URLPath: "/socket.io/?EIO=3&transport=websocket",
	})

	fmt.Println("Init libp2p node")
	h, err := libp2p.New(ctx,
		libp2p.Identity(privKey),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(transport),
		libp2p.ListenAddrs(starMultiaddr1, starMultiaddr2),
		libp2p.Peerstore(peerstore),
		libp2p.Muxer("/yamux/1.0.0", muxer),
	)
	if err != nil {
		return err
	}

	ma, _ := multiaddr.NewMultiaddr("/ip4/159.69.43.228/tcp/4001/p2p/QmSKVUFAyCddg2wDUdZVCfvqG5YCwwJTWY1HRmorebXcKG")

	info, _ := peer2.AddrInfoFromP2pAddr(ma)

	err = h.Connect(ctx, *info)
	if err != nil {
		fmt.Println("error connecting to my bs node")
	}

	fmt.Println("connecting to bootstrap peer")
	for _, peer := range kaddht.GetDefaultBootstrapPeerAddrInfos() {
		if err = h.Connect(ctx, peer); err != nil {
			fmt.Println("error connecting to bootstrap peer", peer)
		}
	}

	fmt.Println("bootstrapping done")

	for {
		time.Sleep(time.Second)
		fmt.Println(peerstore.Peers())
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}

	return nil
}
