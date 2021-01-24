package send

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dennis-tra/pcp/pkg/node"
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

	msg, err := n.NewSendRequest(f.Name(), fstat.Size(), c)
	if err != nil {
		return
	}

	fmt.Print("Asking for confirmation... ")
	respChan, err := n.SendRequest(ctx, pi.ID, msg)
	if err != nil {
		return
	}

	var sendResponse *node.SendResponseData
	select {
	case sendResponse = <-respChan:
	case <-time.NewTicker(60 * time.Second).C:
		err = fmt.Errorf("didn't receive response in time")
		return
	}

	accepted = sendResponse.Response.Accept
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
