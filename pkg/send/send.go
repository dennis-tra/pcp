package send

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"github.com/dennis-tra/pcp/pkg/node"
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

func send(pi peer.AddrInfo, filepath string) (accepted bool, err error) {

	ctx := context.Background()

	n, err := node.InitSending(ctx, pi)
	if err != nil {
		return
	}
	defer n.Close()

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
	err = n.Send(msg)
	if err != nil {
		return
	}

	resp, err := n.Receive()
	if err != nil {
		return
	}

	sendResponse, ok := resp.Payload.(*p2p.MessageData_SendResponse)
	if !ok {
		err = fmt.Errorf("could not decode peer response")
		return
	}

	accepted = sendResponse.SendResponse.Accepted
	if !accepted {
		fmt.Println("Rejected!")
		return
	}

	fmt.Println("Accepted!")
	err = n.SendBytes(f)
	if err != nil {
		return
	}

	fmt.Println("Successfully sent file!")
	return
}
