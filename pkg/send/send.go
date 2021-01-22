package send

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	mh "github.com/multiformats/go-multihash"
)

func send(pi peer.AddrInfo, f *os.File) (accepted bool, err error) {

	ctx := context.Background()

	n, err := node.InitSending(ctx, pi)
	if err != nil {
		return
	}
	defer n.Close()

	msg, err := n.NewMessageData()
	if err != nil {
		return
	}

	stat, err := f.Stat()
	if err != nil {
		return
	}

	s := sha256.New()
	_, err = io.Copy(s, f)
	if err != nil {
		return
	}

	mm, err := mh.Encode(s.Sum(nil), mh.SHA2_256)
	if err != nil {
		return
	}

	msg.Payload = &p2p.MessageData_SendRequest{
		SendRequest: &p2p.SendRequest{
			FileName: f.Name(),
			FileSize: stat.Size(),
			Cid:      cid.NewCidV1(cid.Raw, mm).Bytes(),
		},
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

	time.Sleep(time.Second)
	fmt.Println("Successfully sent file!")
	return
}
