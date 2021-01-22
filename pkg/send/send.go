package send

import (
	"context"
	"fmt"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"os"
	"time"

	"github.com/dennis-tra/pcp/pkg/node"
	"github.com/libp2p/go-libp2p-core/peer"
)

func send(pi peer.AddrInfo, f *os.File) error {

	fmt.Println("Selected peer: ", pi.ID)

	fmt.Print("Establishing connection... ")

	ctx := context.Background()

	n, err := node.InitSending(ctx, pi)
	if err != nil {
		return err
	}
	defer n.Close()

	fmt.Println("Done!")

	msg, err := n.NewMessageData()
	if err != nil {
		return err
	}

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	msg.Payload = &p2p.MessageData_SendRequest{
		SendRequest: &p2p.SendRequest{
			FileName: f.Name(),
			FileSize: stat.Size(),
		},
	}

	fmt.Print("Asking for confirmation... ")
	err = n.Send(msg)
	if err != nil {
		return err
	}

	resp, err := n.Receive()
	if err != nil {
		return err
	}

	if !resp.Payload.(*p2p.MessageData_SendResponse).SendResponse.Accepted {
		fmt.Println("Rejected!")
		return fmt.Errorf("peer rejected your request")
	}

	fmt.Println("Accepted!")
	err = n.SendBytes(f)
	if err != nil {
		return err
	}

	time.Sleep(time.Second)
	fmt.Println("Successfully sent file!")

	return nil
}
