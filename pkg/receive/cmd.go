package receive

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/format"
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/config"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// Command contains the receive sub-command configuration.
var Command = &cli.Command{
	Name:    "receive",
	Usage:   "waits until a peer attempts to connect",
	Aliases: []string{"r"},
	Action:  Action,
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:    "port",
			EnvVars: []string{"PCP_PORT"},
			Aliases: []string{"p"},
			Usage:   "The port at which you are reachable for other peers in the network.",
			Value:   44044,
		},
		&cli.StringFlag{
			Name:    "host",
			EnvVars: []string{"PCP_HOST"},
			Usage:   "The host at which you are reachable for other peers in the network.",
			Value:   "0.0.0.0",
		},
	},
	ArgsUsage: "[DEST_DIR]",
	UsageText: `DEST_DIR	The destination directory where the received file
	should be saved. The file will be named as the sender
	specifies. If no DEST_DIR is given the file will be
	saved to $XDG_DATA_HOME - usually ~/.data/. If the file
	already exists you will be prompted what you want to do.`,
	Description: `The receive subcommand starts a multicast DNS service. This
makes it possible for other peers to discover us - it enables
peer-to-peer discovery. It is important to note that many
networks restrict the use of multicasting, which prevents mDNS
from functioning. Notably, multicast cannot be used in any
sort of cloud, or shared infrastructure environment. However it
works well in most office, home, or private infrastructure
environments.`,
}

// Action is the function that is called when running pcp receive.
func Action(c *cli.Context) error {
	ctx, err := config.FillContext(c.Context)
	if err != nil {
		return errors.Wrap(err, "failed loading configuration")
	}

	local, err := InitNode(ctx, c.String("host"), c.Int64("port"))
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to initialize node"))
	}
	defer local.Close()

	log.Infof("Your identity:\n\n\t%s\n\n", local.Host.ID())

	err = local.StartMdnsService(ctx)
	if err != nil {
		return err
	}

	for {
		log.Infoln("Ready to receive files... (cancel with strg+c)")
		peerId, pushRequest := local.WaitForPushRequest()
		log.Infof("Sending request: %s (%s)\n", pushRequest.FileName, format.Bytes(pushRequest.FileSize))

		quit, err := handlePushRequest(ctx, local, peerId, pushRequest)
		if quit {
			return err
		}

		if err != nil {
			log.Infoln(err)
		}
	}
}

func handlePushRequest(ctx context.Context, local *Node, peerId peer.ID, pushRequest *p2p.PushRequest) (bool, error) {
	for {
		log.Infoln("Do you want to receive this file? [y,n,i,q,?] ")
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
			return true, nil
		}

		// Print the help text and prompt again
		if input == "?" {
			help()
			continue
		}

		// Print information about the send request
		if input == "i" {
			printInformation(pushRequest)
			continue
		}

		// Accept the file transfer
		if input == "y" {
			return true, local.Accept(ctx, peerId, pushRequest)
		}

		// Reject the file transfer
		if input == "n" {
			return false, local.Reject(ctx, peerId, pushRequest)
		}

		log.Infoln("Invalid input")
	}
}

func printInformation(data *p2p.PushRequest) {

	var cStr string
	if c, err := cid.Cast(data.Cid); err != nil {
		cStr = err.Error()
	} else {
		cStr = c.String()
	}

	log.Infoln("Sending request information:")
	log.Infoln("\tPeer:\t", data.Header.NodeId)
	log.Infoln("\tName:\t", data.FileName)
	log.Infoln("\tSize:\t", data.FileSize)
	log.Infoln("\tCID:\t", cStr)
	log.Infoln("\tSign:\t", hex.EncodeToString(data.Header.Signature))
	log.Infoln("\tPubKey:\t", hex.EncodeToString(data.Header.GetNodePubKey()))
}

func help() {
	log.Infoln("y: accept and thus accept the file")
	log.Infoln("n: reject the request to accept the file")
	log.Infoln("i: show information about the sender and file to be received")
	log.Infoln("q: quit pcp")
	log.Infoln("?: this help message")
}
