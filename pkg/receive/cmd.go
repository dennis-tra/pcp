package receive

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/node"
	"github.com/ipfs/go-cid"
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

	conf, err := config.LoadConfig()
	if err != nil {
		return err
	}

	ctx := context.WithValue(c.Context, config.ContextKey, conf)

	n, err := InitNode(ctx, c.String("host"), c.Int64("port"))
	if err != nil {
		return err
	}
	defer n.Close()

	fmt.Printf("Your identity:\n\n\t%s\n\n", n.Host.ID())

	err = n.StartMdnsService(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Ready to receive files... (cancel with strg+c)")
	req := <-n.ReqChan

	err = printSendRequest(req)
	if err != nil {
		return err
	}

	for {

		fmt.Printf("Do you want to receive this file? [y,n,q,?] ")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return scanner.Err()
		}

		input := strings.ToLower(strings.TrimSpace(scanner.Text()))

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
			return receive(ctx, n, req)
		}

		if input == "n" {
			resp, err := n.NewSendResponse(false)
			if err != nil {
				return err
			}

			err = n.SendResponse(ctx, req, resp)
			if err != nil {
				return err
			}

			fmt.Println("Ready to receive files... (cancel with strg+c)")
			req = <-n.ReqChan

			err = printSendRequest(req)
			if err != nil {
				return err
			}

			continue
		}

		fmt.Println("Invalid input")
		err = printSendRequest(req)
		if err != nil {
			return err
		}
	}
}

func receive(ctx context.Context, n *Node, req *node.SendRequestData) error {

	// TODO: better handling.
	filename := filepath.Base(req.Request.FileName)
	_, err := os.Stat(req.Request.FileName)
	if os.IsExist(err) {
		filename += "_2"
	}

	fmt.Println("Saving file to", filename)
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	receiveChan := n.TransferProtocol.SetExpectedData(req, f)
	defer n.TransferProtocol.ResetExpectedData() // TODO: possible resource leak

	resp, err := n.NewSendResponse(true)
	if err != nil {
		return err
	}

	fmt.Println("Sending confirmation message...")
	err = n.SendResponse(ctx, req, resp)
	if err != nil {
		return err
	}

	fmt.Println("Waiting to receive data")
	<-receiveChan

	return nil
}

func printSendRequest(data *node.SendRequestData) error {

	c, err := cid.Cast(data.Request.Cid)
	if err != nil {
		return err
	}

	fmt.Println("Sending request:")
	fmt.Println("  Peer:\t", data.Header.NodeId)
	fmt.Println("  Name:\t", data.Request.FileName)
	fmt.Println("  Size:\t", data.Request.FileSize)
	fmt.Println("  CID:\t", c.String())

	return nil
}

func help() {
	fmt.Println("y: accept and thus receive the file")
	fmt.Println("n: reject the request to receive the file")
	fmt.Println("q: quit pcp")
	fmt.Println("?: this help message")
}
