package receive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

type Node struct {
	*node.Node
}

func InitNode(ctx context.Context, host string, port int64) (*Node, error) {

	hostAddr := fmt.Sprintf("/ip4/%s/tcp/%d", host, port)
	nn, err := node.Init(ctx, libp2p.ListenAddrStrings(hostAddr))
	if err != nil {
		return nil, err
	}

	n := &Node{nn}

	return n, nil
}

func (n *Node) Close() error {
	err := n.Host.Close()
	if err != nil {
		log.Infoln(err)
	}

	err = n.StopMdnsService()
	if err != nil {
		log.Infoln(err)
	}

	return nil
}

func (n *Node) Accept(ctx context.Context, peerId peer.ID, req *p2p.PushRequest) error {

	filename := filepath.Base(req.FileName)

	// TODO: Handle that the file may already exist.

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	log.Infoln("Saving file to: ", filepath.Join(cwd, filename))
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	go func() {
		_, err = n.SendProtoWithParentId(ctx, peerId, p2p.NewPushResponse(true), req.Header.RequestId)
		if err != nil {
			log.Infoln(err)
			// TODO: When this fails we need to skip the receive call.
		}
	}()
	receivedBytes, err := n.Receive(ctx, peerId, req, f)
	if err != nil {
		return err
	}

	if receivedBytes == req.FileSize {
		log.Infoln("Successfully received file!")
	} else {
		log.Infof("WARNING: Only received %d from %d bytes!", receivedBytes, req.FileSize)
	}

	return nil
}

func (n *Node) Reject(ctx context.Context, peerId peer.ID, pushRequest *p2p.PushRequest) error {
	_, err := n.SendProtoWithParentId(ctx, peerId, p2p.NewPushResponse(false), pushRequest.Header.RequestId)
	return err
}
