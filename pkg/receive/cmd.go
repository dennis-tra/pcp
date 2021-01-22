package receive

import (
	"bufio"
	"context"
	"fmt"
	"github.com/dennis-tra/pcp/pkg/node"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/urfave/cli/v2"
	"os"
	"strings"
	"time"
)

// Command contains the receive sub-command configuration.
var Command = &cli.Command{
	Name:    "receive",
	Aliases: []string{"r"},
	Action:  Action,
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:    "port",
			EnvVars: []string{"PCP_PORT"},
			Value:   44044,
		},
	},
}

// Action is the function that is called when
// running pcp receive.
func Action(c *cli.Context) error {

	ctx := context.Background()

	n, err := node.InitReceiving(ctx, c.Int64("port"))
	if err != nil {
		return err
	}
	defer n.Close()

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

		fmt.Println("Received unsupported message")
	}

	for {
		fmt.Printf("Peer %s wants to send you the file %s (%d). Accept? [y,n,q,?] ", msgData.NodeId, sendRequest.FileName, sendRequest.FileSize)
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
			time.Sleep(time.Second)
			break
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
