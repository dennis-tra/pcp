package receive

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/urfave/cli/v2"
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

	ctx := context.Background()
	n, err := node.InitReceiving(ctx, c.Int64("port"))
	if err != nil {
		return err
	}
	defer n.Close()

	fmt.Println("Your identity:", n.ID())
	fmt.Println("Waiting for peers to connect... (cancel with strg+c)")

	n.WaitForConnection()

	var sendRequest *p2p.SendRequest
	var msgData *p2p.MessageData
	for {
		msgData, err = n.Receive()
		if err != nil {
			return err
		}

		if msgDatSendReq, ok := msgData.Payload.(*p2p.MessageData_SendRequest); ok {
			sendRequest = msgDatSendReq.SendRequest
			break
		}

		fmt.Println("Warning: Received unsupported message from peer:", msgData.NodeId)
	}

	for {
		fmt.Printf("Peer %s wants to send you the file %s (%d Bytes). Accept? [y,n,q,?] ", msgData.NodeId, sendRequest.FileName, sendRequest.FileSize)
		scanner := bufio.NewScanner(os.Stdin)
		scanResult := scanner.Scan()

		if !scanResult {
			return scanner.Err()
		}

		input := strings.TrimSpace(scanner.Text())

		// Empty input, user just pressed enter => do nothing and prompt again
		if input == "" {
			continue
		}

		// Quit the process
		if input == "q" {
			return nil
		}

		// Print the help text and prompt again
		if input == "?" {
			help()
			continue
		}

		// Print the help text and prompt again
		if input == "y" {

			filename := filepath.Base(sendRequest.FileName)
			_, err := os.Stat(sendRequest.FileName)
			if os.IsExist(err) {
				filename += "_2"
			}

			resp, err := n.NewMessageData()
			if err != nil {
				return err
			}

			resp.Payload = &p2p.MessageData_SendResponse{
				SendResponse: &p2p.SendResponse{
					Accepted: true,
				},
			}

			err = n.Send(resp)
			if err != nil {
				return err
			}

			fmt.Println("Saving file to: ", filename)
			err = n.ReceiveBytes(filename)
			if err != nil {
				return err
			}

			fmt.Println("Successfully received file!")

			return nil
		}

		if input == "n" {
			resp, err := n.NewMessageData()
			if err != nil {
				return err
			}

			resp.Payload = &p2p.MessageData_SendResponse{
				SendResponse: &p2p.SendResponse{
					Accepted: false,
				},
			}

			err = n.Send(resp)
			if err != nil {
				return err
			}
			break
		}

		fmt.Println("Invalid input")
	}

	return nil
}

func help() {
	fmt.Printf("Help text")
}
