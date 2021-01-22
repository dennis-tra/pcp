package receive

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/network"
	protocol "github.com/libp2p/go-libp2p-protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
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

	listenIP := "127.0.0.1"
	listenPort := c.Int64("port")

	ctx := context.Background()

	sourceAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", listenIP, listenPort))
	if err != nil {
		return err
	}

	host, err := libp2p.New(ctx, libp2p.ListenAddrs(sourceAddr))
	if err != nil {
		return err
	}

	host.SetStreamHandler(protocol.ID("pcp"), handleStream)

	fmt.Println("Your identity: ", host.ID().Pretty())

	ser, err := discovery.NewMdnsService(ctx, host, time.Second, "pcp")
	if err != nil {
		return err
	}
	defer ser.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)

	select {
	case <-stop:
		host.Close()
		os.Exit(0)
	}

	return nil
}

func handleStream(stream network.Stream) {
	fmt.Println("Got a new stream!")

	// Create a buffer stream for non blocking read and write.
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	go readData(rw)
}

func readData(rw *bufio.ReadWriter) {
	f, err := os.Create("LICENSE2")
	if err != nil {
		panic(err)
	}
	for {
		b, err := rw.ReadByte()
		if err != nil {
			panic(err)
		}

		_, err = f.Write([]byte{b})
		if err != nil {
			panic(err)
		}
	}
}
