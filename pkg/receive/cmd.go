package receive

import (
	"encoding/hex"
	"fmt"
	"github.com/dennis-tra/pcp/pkg/words"
	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"strings"
	"time"

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

	// TODO: make words count configurable
	tcode := strings.Split(c.Args().First(), "-") // transfer code
	if len(tcode) != 4 {
		return fmt.Errorf("list of words must be exactly 4")
	}

	chanID, err := words.ToInt(tcode[0])
	if err != nil {
		return err
	}

	local, err := InitNode(ctx, tcode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to initialize node"))
	}

	// Search for identifier
	dhtKey := local.AdvertiseIdentifier(time.Now(), chanID)
	log.Infof("Looking for peer %s... (%s)\n", c.Args().First(), dhtKey)
	local.Discover(dhtKey)

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
	log.Infoln("y: accept the file transfer")
	log.Infoln("n: reject the file transfer")
	log.Infoln("i: show information about the sender and file to be received")
	log.Infoln("?: this help message")
}
