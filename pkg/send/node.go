package send

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

type Node struct {
	*node.Node
}

func InitNode(ctx context.Context) (*Node, error) {

	n, err := node.Init(ctx)
	if err != nil {
		return nil, err
	}

	return &Node{n}, nil
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

func (n *Node) Transfer(ctx context.Context, pi peer.AddrInfo, filepath string) (accepted bool, err error) {

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

	msg := p2p.NewPushRequest(path.Base(f.Name()), fstat.Size(), c)
	log.Infof("Asking for confirmation... ")

	sendResponse, err := n.SendPushRequest(ctx, pi.ID, msg)
	if err != nil {
		return
	}

	accepted = sendResponse.Accept
	if !accepted {
		log.Infoln("Rejected!")
		return
	}

	log.Infoln("Accepted!")

	acknowledged, err := n.Node.Transfer(ctx, pi.ID, f, msg.FileName, msg.FileSize)
	if err != nil {
		err = errors.Wrap(err, "could not transfer file to peer")
		return
	}

	if fstat.Size() != acknowledged {
		err = fmt.Errorf("copied %d bytes, but acknowledged were %d bytes", fstat.Size(), acknowledged)
		return
	}

	log.Infoln("Successfully sent file!")
	return
}
