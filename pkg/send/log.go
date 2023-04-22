package send

import (
	"strconv"
	"strings"
	"time"

	"github.com/dennis-tra/pcp/pkg/dht"

	"github.com/dennis-tra/pcp/pkg/mdns"

	"github.com/libp2p/go-libp2p/core/network"

	"github.com/dennis-tra/pcp/internal/log"
)

var spinnerChars = []string{"⠋ ", "⠙ ", "⠹ ", "⠸ ", "⠼ ", "⠴ ", "⠦ ", "⠧ ", "⠇ ", "⠏ "}

func (n *Node) logLoop(verbose bool) {
	// Broadcast the code to be found by peers.
	log.Infoln("On the other machine run:\n\tpeercp receive", strings.Join(n.Words, "-"))
	log.Infoln("")

	n.logLoopWg.Add(1)

	n.statusTicker = time.NewTicker(100 * time.Millisecond)
	go func() {
		defer n.logLoopWg.Done()

		as := n.advertiseStatus(spinnerChars[0])

		log.AdvertiseStatus(as, verbose)

		for i := 0; ; i++ {
			spinnerChar := spinnerChars[i%len(spinnerChars)]

			as = n.advertiseStatus(spinnerChar)

			select {
			case <-n.ServiceContext().Done():
				return
			case _, ok := <-n.statusTicker.C:
				select {
				case <-n.ServiceContext().Done():
					return
				default:
				}

				log.EraseAdvertiseStatus(verbose)
				log.AdvertiseStatus(as, verbose)

				if !ok {
					return
				}
			}
		}
	}()
}

func (n *Node) advertiseStatus(spinnerChar string) log.AdvertiseStatusParams {
	mdnsState := n.mdnsAdvertiser.State()
	dhtState := n.dhtAdvertiser.State()
	asp := log.AdvertiseStatusParams{
		Code:         strings.Join(n.Words, "-"),
		LANState:     log.Gray("-"),
		MDNSState:    log.Gray("-"),
		Reachability: log.Gray("-"),
		RelayAddrs:   log.Gray("-"),
	}

	switch mdnsState {
	case mdns.StateIdle:
		asp.MDNSState = log.Gray("-")
		asp.LANState = log.Green("-")
	case mdns.StateAdvertising:
		asp.MDNSState = log.Green("active")
		asp.LANState = log.Green("ready")
	case mdns.StateError:
		asp.MDNSState = log.Red(n.mdnsAdvertiser.Error().Error())
		asp.LANState = log.Red("failed")
	case mdns.StateStopped:
		asp.MDNSState = log.Green("inactive")
		asp.LANState = log.Green("stopped")
	}

	switch dhtState.Stage {
	case dht.StageIdle:
		asp.DHTState = log.Gray("-")
		asp.WANState = log.Green("-")
	case dht.StageBootstrapping:
		asp.DHTState = spinnerChar + "(bootstrapping)"
		asp.WANState = spinnerChar + "(bootstrapping)"
	case dht.StageCheckingNetwork:
		if n.relayFinderActive {
			asp.DHTState = spinnerChar + "(finding relays)"
			asp.WANState = spinnerChar + "(finding relays)"
		} else {
			asp.DHTState = spinnerChar + "(analyzing network)"
			asp.WANState = spinnerChar + "(analyzing network)"
		}
	case dht.StageProviding:
		asp.DHTState = spinnerChar + "(providing)"
		asp.WANState = spinnerChar + "(writing to DHT)"
	case dht.StageRetrying:
		asp.DHTState = spinnerChar + "(retrying)"
		asp.WANState = spinnerChar + log.Yellow("(retry writing to DHT)")
	case dht.StageProvided:
		asp.DHTState = log.Green("active")
		asp.WANState = log.Green("ready")
	case dht.StageError:
		asp.DHTState = log.Red(n.dhtAdvertiser.Error().Error())
		asp.WANState = log.Red("failed (" + n.dhtAdvertiser.Error().Error() + ")")
	case dht.StageStopped:
		asp.DHTState = log.Green("inactive")
		asp.WANState = log.Green("stopped")
	}

	switch dhtState.Reachability {
	case network.ReachabilityUnknown:
		asp.Reachability = spinnerChar
	case network.ReachabilityPrivate:
		asp.Reachability = log.Yellow(strings.ToLower(dhtState.Reachability.String()))
	case network.ReachabilityPublic:
		asp.Reachability = log.Green(strings.ToLower(dhtState.Reachability.String()))
	}

	asp.NATState = ""
	switch dhtState.NATTypeUDP {
	case network.NATDeviceTypeUnknown:
		if dhtState.Reachability == network.ReachabilityPublic {
			asp.NATState += log.Gray("irrelevant")
		} else {
			asp.NATState += spinnerChar
		}
	case network.NATDeviceTypeCone:
		asp.NATState += log.Green(strings.ToLower(dhtState.NATTypeUDP.String()))
	case network.NATDeviceTypeSymmetric:
		asp.NATState += log.Red(strings.ToLower(dhtState.NATTypeUDP.String()))
	}
	asp.NATState += " / "
	switch dhtState.NATTypeTCP {
	case network.NATDeviceTypeUnknown:
		if dhtState.Reachability == network.ReachabilityPublic {
			asp.NATState += log.Gray("irrelevant")
		} else {
			asp.NATState += spinnerChar + " "
		}
	case network.NATDeviceTypeCone:
		asp.NATState += log.Green(strings.ToLower(dhtState.NATTypeTCP.String()))
	case network.NATDeviceTypeSymmetric:
		asp.NATState += log.Red(strings.ToLower(dhtState.NATTypeTCP.String()))
	}

	if dhtState.Reachability == network.ReachabilityPublic {
		asp.RelayAddrs = log.Gray("irrelevant")
	} else if n.relayFinderActive {
		asp.RelayAddrs = spinnerChar
		if len(dhtState.RelayAddrs) > 0 {
			asp.RelayAddrs = log.Green(strconv.Itoa(len(dhtState.RelayAddrs)))
		}
	}

	if len(dhtState.PrivateAddrs) == 0 {
		asp.PrivateAddrs = log.Red("0")
	} else {
		asp.PrivateAddrs = log.Green(strconv.Itoa(len(dhtState.PrivateAddrs)))
	}

	if len(dhtState.PublicAddrs) == 0 {
		if len(dhtState.RelayAddrs) > 0 {
			asp.PublicAddrs = log.Gray("0")
		} else {
			asp.PublicAddrs = spinnerChar
		}
	} else {
		asp.PublicAddrs = log.Green(strconv.Itoa(len(dhtState.PublicAddrs)))
	}

	return asp
}
