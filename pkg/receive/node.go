package receive

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/libp2p/go-libp2p"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	"github.com/dennis-tra/pcp/internal/format"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

type Node struct {
	*node.Node
	busy     *atomic.Bool
	shutdown chan error
}

func InitNode(ctx context.Context, host string, port int64, shutdown chan error) (*Node, error) {

	hostAddr := fmt.Sprintf("/ip4/%s/tcp/%d", host, port)
	nn, err := node.Init(ctx, libp2p.ListenAddrStrings(hostAddr))
	if err != nil {
		return nil, err
	}

	n := &Node{
		Node:     nn,
		busy:     atomic.NewBool(false),
		shutdown: shutdown,
	}
	n.RegisterRequestHandler(n)

	return n, nil
}

func (n *Node) Shutdown(err error) {
	n.shutdown <- err
	close(n.shutdown)
}

func (n *Node) HandlePushRequest(pr *p2p.PushRequest) (bool, error) {
	if n.busy.Load() {
		return false, nil
	}
	n.busy.Store(true)

	log.Infof("Sending request: %s (%s)\n", pr.Filename, format.Bytes(pr.Size))
	for {
		log.Infof("Do you want to receive this file? [y,n,i,q,?] ")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return true, errors.Wrap(scanner.Err(), "failed reading from stdin")
		}

		// sanitize user input
		input := strings.ToLower(strings.TrimSpace(scanner.Text()))

		// Empty input, user just pressed enter => do nothing and prompt again
		if input == "" {
			continue
		}

		// Quit the process
		if input == "q" {
			go n.Shutdown(nil)
			return false, nil
		}

		// Print the help text and prompt again
		if input == "?" {
			help()
			continue
		}

		// Print information about the send request
		if input == "i" {
			printInformation(pr)
			continue
		}

		// Accept the file transfer
		if input == "y" {

			peerID, err := pr.PeerID()
			if err != nil {
				return true, err
			}

			done := n.TransferFinishHandler(pr.Size)
			th, err := NewTransferHandler(peerID, pr.Filename, pr.Size, pr.Cid, done)
			if err != nil {
				return true, err
			}
			n.RegisterTransferHandler(th)

			return true, nil
		}

		// Reject the file transfer
		if input == "n" {
			n.busy.Store(false)
			log.Infoln("Ready to receive files... (cancel with ctrl+c)")
			return false, nil
		}

		log.Infoln("Invalid input")
	}
}

func (n *Node) TransferFinishHandler(size int64) chan int64 {
	done := make(chan int64)
	go func() {
		var received int64
		select {
		case received = <-done:
		case <-n.shutdown:
			return
		}

		if received == size {
			log.Infoln("Successfully received file!")
		} else {
			log.Infoln("WARNING: Only received %d of %d bytes!", received, size)
		}

		n.shutdown <- nil
	}()
	return done
}
