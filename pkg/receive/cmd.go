package receive

import (
	"encoding/hex"
	"fmt"
	"github.com/dennis-tra/pcp/pkg/term"
	"github.com/tyler-smith/go-bip39/wordlists"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/config"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// Command contains the receive sub-command configuration.
var Command = &cli.Command{
	Name:      "receive",
	Usage:     "searches for the given code for peers in the local network and DHT",
	Aliases:   []string{"r"},
	Action:    Action,
	ArgsUsage: "[WORD-CODE]",
	UsageText: `WORD-CODE	`,
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

	local, err := InitNode(ctx)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to initialize node"))
	}
	defer local.Close()

	words := strings.Split(c.Args().First(), "-")
	if len(words) != 4 {
		return fmt.Errorf("list of words must be exactly 4")
	}

	chanID, err := intForWord(words[0])
	if err != nil {
		return err
	}

	// Search for identifier
	local.Discover(ctx, local.AdvertiseIdentifier(time.Now(), chanID))

	// Wait for the user to stop the tool
	term.Wait(ctx)

	return nil
}

func intForWord(word string) (int, error) {
	for i, w := range wordlists.English {
		if w == word {
			return i, nil
		}
	}
	return 0, fmt.Errorf("word not found in list")
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
	log.Infoln("\tName:\t", data.Filename)
	log.Infoln("\tSize:\t", data.Size)
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
