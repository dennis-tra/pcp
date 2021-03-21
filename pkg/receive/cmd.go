package receive

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/config"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// Command contains the receive sub-command configuration.
var Command = &cli.Command{
	Name:      "receive",
	Usage:     "search for peers in your local network and the DHT",
	Aliases:   []string{"r"},
	Action:    Action,
	ArgsUsage: "[WORD-CODE]",
	Description: `The receive subcommand starts searching for peers in your local 
network by sending out multicast DNS queries. These queries are
based on the current time and the first word of the given list. It
simultaneously queries the distributed hash table (DHT) with the
exact same parameters.

It is important to note that many networks restrict the use of
multicasting, which prevents mDNS from functioning. Notably,
multicast cannot be used in any sort of cloud, or shared infra-
structure environment. However it works well in most office, home,
or private infrastructure environments.

After it has found a potential peer it starts a password authen-
ticated key exchange (PAKE) with the remaining three words to
proof that the peer is in possession of the password. While this
is happening the tool still searches for other peers as the
currently connected one could fail the authentication procedure.

After the authentication was successful you need to confirm the
file transfer. The confirmation dialog shows the name and size of
the file.

The file will be saved to your current working directory overwriting
any files with the same name. If the transmission fails the file 
will contain the partial written bytes.`,
}

// Action is the function that is called when running pcp receive.
func Action(c *cli.Context) error {
	ctx, err := config.FillContext(c.Context)
	if err != nil {
		return errors.Wrap(err, "failed loading configuration")
	}

	words := strings.Split(c.Args().First(), "-") // transfer words

	local, err := InitNode(ctx, words)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to initialize node"))
	}

	// Search for identifier
	log.Infof("Looking for peer %s... \n", c.Args().First())
	local.StartDiscovering(c)

	// Wait for the user to stop the tool or the transfer to finish.
	select {
	case <-ctx.Done():
		local.Shutdown()
		return nil
	case <-local.SigDone():
		return nil
	}
}

func printInformation(data *p2p.PushRequest) {
	log.Infoln("Sending request information:")
	log.Infoln("\tPeer:\t", data.Header.NodeId)
	log.Infoln("\tName:\t", data.Name)
	log.Infoln("\tSize:\t", data.Size)
	log.Infoln("\tSign:\t", hex.EncodeToString(data.Header.Signature))
	log.Infoln("\tPubKey:\t", hex.EncodeToString(data.Header.GetNodePubKey()))
}

func help() {
	log.Infoln("y: accept the file transfer")
	log.Infoln("n: reject the file transfer")
	log.Infoln("i: show information about the sender and file to be received")
	log.Infoln("?: this help message")
}
