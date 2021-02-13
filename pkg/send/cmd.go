package send

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/config"
)

// Command holds the `send` subcommand configuration.
var Command = &cli.Command{
	Name:      "send",
	Usage:     "make the given file available to your peers",
	Aliases:   []string{"s"},
	Action:    Action,
	Flags:     []cli.Flag{},
	ArgsUsage: `FILE`,
	// UsageText: `pcp send [command options] FILE
	// The file you want to transmit to your peer (required).`,
	Description: `
The send subcommand generates four random words based on the first
bytes of a newly generated peer identity. The first word and the
current time are used to generate an identifier that is broadcasted
in your local network via mDNS and provided through the distributed
hash table of the IPFS network.

After a peer attempts to connect it starts a password authen-
ticated key exchange (PAKE) with the remaining three words to
proof that the peer is in possession of the password. While this
is happening the tool still searches for other peers as the
currently connected one could fail the authentication procedure.

After the authentication was successful and the peer confirmed
the file transfer the transmission is started.`,
}

// Action contains the logic for the send subcommand of the pcp program. It is
// mainly responsible for the TUI state handling and input parsing.
func Action(c *cli.Context) error {
	// Read config file and fill context with it.
	ctx, err := config.FillContext(c.Context)
	if err != nil {
		return err
	}

	// Try to open the file to check if we have access and fail early.
	filepath := c.Args().First()
	if err = verifyFileAccess(filepath); err != nil {
		return err
	}

	// Initialize node
	local, err := InitNode(ctx, filepath)
	if err != nil {
		return err
	}

	// Broadcast the code to be found by peers.
	dhtKey := local.AdvertiseIdentifier(time.Now(), local.ChannelID)
	log.Infoln("Code is: ", strings.Join(local.TransferCode, "-"))
	log.Infoln("On the other machine run:\n\tpcp receive", strings.Join(local.TransferCode, "-"))

	local.Advertise(dhtKey)

	// Wait for the user to stop the tool or the transfer to finish.
	select {
	case <-ctx.Done():
		local.Shutdown()
		return nil
	case <-local.SigDone():
		return nil
	}
}

// verifyFileAccess just tries to open the file at the given path to check
// if we have the correct permissions to read it.
func verifyFileAccess(filepath string) error {
	if filepath == "" {
		return fmt.Errorf("please specify the file you want to transfer")
	}

	f, err := os.Open(filepath)
	if err != nil {
		return err
	}

	return f.Close()
}

// help prints the usage description for the user input in the "select peer" prompt.
func help() {
	log.Infoln("#: the number of the peer you want to connect to")
	log.Infoln("r: refresh peer list")
	log.Infoln("q: quit pcp")
	log.Infoln("?: this help message")
}

func calcContentID(filepath string) (cid.Cid, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return cid.Cid{}, err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return cid.Cid{}, err
	}

	mhash, err := mh.Encode(hasher.Sum(nil), mh.SHA2_256)
	if err != nil {
		return cid.Cid{}, err
	}

	return cid.NewCidV1(cid.Raw, mhash), nil
}
