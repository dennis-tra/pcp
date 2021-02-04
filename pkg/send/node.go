package send

import (
	"context"
	"os"
	"path"
	"sync"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/node"
	"github.com/dennis-tra/pcp/pkg/progress"
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

func (n *Node) Transfer(ctx context.Context, pi peer.AddrInfo, filepath string) (bool, error) {

	if err := n.Connect(ctx, pi); err != nil {
		return false, err
	}

	c, err := calcContentID(filepath)
	if err != nil {
		return false, err
	}

	f, err := os.Open(filepath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	fstat, err := f.Stat()
	if err != nil {
		return false, err
	}

	log.Infof("Asking for confirmation... ")

	accepted, err := n.SendPushRequest(ctx, pi.ID, path.Base(f.Name()), fstat.Size(), c)
	if err != nil {
		return false, err
	}

	if !accepted {
		log.Infoln("Rejected!")
		return accepted, nil
	}
	log.Infoln("Accepted!")

	pr := progress.NewReader(f)

	var wg sync.WaitGroup
	wg.Add(1)

	ctx, cancel := context.WithCancel(context.Background())
	go node.IndicateProgress(ctx, pr, path.Base(f.Name()), fstat.Size(), &wg)
	defer func() { cancel(); wg.Wait() }()

	if _, err = n.Node.Transfer(ctx, pi.ID, pr); err != nil {
		return accepted, errors.Wrap(err, "could not transfer file to peer")
	}

	log.Infoln("Successfully sent file!")
	return accepted, nil
}
