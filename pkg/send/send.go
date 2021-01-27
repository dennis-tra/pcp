package send

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	mh "github.com/multiformats/go-multihash"
)

func calcContentID(filepath string) (cid.Cid, error) {

	f, err := os.Open(filepath)
	if err != nil {
		return cid.Cid{}, err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return cid.Cid{}, err
	}

	mhash, err := mh.Encode(hasher.Sum(nil), mh.SHA2_256)
	if err != nil {
		return cid.Cid{}, err
	}

	return cid.NewCidV1(cid.Raw, mhash), nil
}

func (n *Node) send(ctx context.Context, pi peer.AddrInfo, filepath string) (accepted bool, err error) {

	err = n.Connect(ctx, pi)
	if err != nil {
		return
	}

	c, err := calcContentID(filepath)
	if err != nil {
		return
	}

	f, err := os.Open(filepath)
	if err != nil {
		return
	}
	defer f.Close()

	fstat, err := f.Stat()
	if err != nil {
		return
	}

	msg, err := p2p.NewPushRequest(path.Base(f.Name()), fstat.Size(), c)
	if err != nil {
		return
	}

	fmt.Print("Asking for confirmation... ")

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 10*time.Second)
	sendResponse, err := n.SendPushRequest(timeoutCtx, pi.ID, msg)
	timeoutCancel()
	if err != nil {
		return
	}

	accepted = sendResponse.Accept
	if !accepted {
		fmt.Println("Rejected!")
		return
	}

	fmt.Println("Accepted!")

	fmt.Print("Transferring file...")
	ackChan, err := n.Transfer(ctx, pi.ID, f)
	if err != nil {
		fmt.Println("Error!")
		return
	}
	fmt.Println("Done!")

	fmt.Print("Awaiting acknowledgment...")
	acknowledged := <-ackChan
	if fstat.Size() != acknowledged {
		err = fmt.Errorf("copied %d bytes, but acked were %d bytes", fstat.Size(), acknowledged)
		return
	}
	fmt.Println("Received!")

	fmt.Println("Successfully sent file!")
	return
}
