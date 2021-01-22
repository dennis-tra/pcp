package send

import (
	"context"
	"fmt"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"os"

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

	msg.Payload = &p2p.MessageData_SendRequest{
		SendRequest: &p2p.SendRequest{
			FileName: "Testfile",
			FileSize: 3,
		},
	}

	fmt.Println("Asking for confirmation...")
	err = n.Send(msg)
	if err != nil {
		return err
	}

	resp, err := n.Receive()
	if err != nil {
		return err
	}

	fmt.Println("Peer replied with", resp.Payload.(*p2p.MessageData_SendResponse).SendResponse.Accepted)

	//dat, err := ioutil.ReadAll(stream)
	//if err != nil {
	//	return err
	//}
	//
	//fmt.Println(dat)

	//finfo, err := f.Stat()
	//if err != nil {
	//	return err
	//}
	//
	//msg := &pb.Message{
	//	Type: &pb.Message_SendRequest{
	//		SendRequest: &pb.SendRequest{
	//			FileName: f.Name(),
	//			FileSize: finfo.Size(),
	//		},
	//	},
	//}
	//
	//msgPayload, err := proto.Marshal(msg)
	//if err != nil {
	//	return err
	//}
	//
	//time.Sleep(time.Second)
	//
	//fmt.Println("Sending ", msgPayload)
	//_, err = rw.Write(msgPayload)
	//if err != nil {
	//	return err
	//}
	//
	//err = rw.Flush()
	//if err != nil {
	//	return err
	//}
	//
	//time.Sleep(time.Second)

	//fmt.Println("Streaming file content to peer...")
	//_, err = io.Copy(stream, f)
	//if err != nil {
	//	return err
	//}

	fmt.Println("Successfully sent file!")

	return nil
}
