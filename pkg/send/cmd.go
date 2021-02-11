package send

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/dennis-tra/pcp/pkg/term"
)

// Command holds the `send` subcommand configuration.
var Command = &cli.Command{
	Name:      "send",
	Usage:     "makes the given file available to peers who then can search for it",
	Aliases:   []string{"s"},
	Action:    Action,
	Flags:     []cli.Flag{},
	ArgsUsage: "FILE",
	UsageText: `FILE	The file you want to transmit to your peer (required).`,
	Description: `The send subcommand will broadcast in your local network that you're 
providing a particular file via multicast DNS services. Furthermore
pcp will advertise your file in the distributed hash table of the 
IPFS network. Other peers will be able to find you and the file
you're providing and request to receive it.`,
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
	local, err := InitNode(ctx)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to initialize node"))
	}
	defer local.Close()

	// Get Transfer code
	code, err := local.TransferCode()
	if err != nil {
		return err
	}

	log.Infoln("Code is: ", strings.Join(code, "-"))
	log.Infoln("On the other machine run:\n\tpcp receive", strings.Join(code, "-"))

	// Get channel ID
	chanID, err := local.ChannelID()
	if err != nil {
		return err
	}

	// Broadcast the code to be found by peers.
	local.Advertise(ctx, local.AdvertiseIdentifier(time.Now(), chanID))

	// Wait for the user to stop the tool
	term.Wait(ctx)

	return nil
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
