package receive

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/config"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// Command contains the `receive` sub-command configuration.
var Command = &cli.Command{
	Name:      "receive",
	Usage:     "search for peers in your local network and the DHT",
	Aliases:   []string{"r", "get"},
	Action:    Action,
	ArgsUsage: "[WORD-CODE]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "auto-accept",
			Aliases: []string{"yes", "y"},
			Usage:   "automatically accept the file transfer",
			EnvVars: []string{"PCP_AUTO_ACCEPT", "PCP_AUTO_ACCEPT" /* legacy */},
		},
	},
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
	// Read config file and fill context with it.
	c, err := config.FillContext(c)
	if err != nil {
		return fmt.Errorf("failed loading configuration: %w", err)
	}

	words := strings.Split(c.Args().First(), "-") // transfer words
	if len(words) < 3 {
		return fmt.Errorf("the number of words must not be less than 3")
	}

	node, err := InitNode(c, words)
	if err != nil {
		return fmt.Errorf("failed to initialize node: %w", err)
	}

	// Search for identifier
	log.Infof("Looking for peer: %s... \n", c.Args().First())

	// if mDNS is active, start searching in the local network
	if isMDNSActive(c) {
		go node.StartDiscoveringMDNS()
	}

	// if mDNS is active, start searching in the wider area network
	if isDHTActive(c) {
		go node.StartDiscoveringDHT()
	}

	// Wait for the user to stop the tool or the transfer to finish.
	select {
	case <-c.Done():
		node.Shutdown()
		return nil
	case <-node.SigDone():
		return nil
	}
}

// isMDNSActive returns true if the user explicitly chose to only advertised via mDNS
// or if they didn't specify any preference.
func isMDNSActive(c *cli.Context) bool {
	return c.Bool("mdns") || c.Bool("dht") == c.Bool("mdns")
}

// isDHTActive returns true if the user explicitly chose to only advertised via the DHT
// or if they didn't specify any preference.
func isDHTActive(c *cli.Context) bool {
	return c.Bool("dht") || c.Bool("dht") == c.Bool("mdns")
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
