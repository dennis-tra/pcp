package log

import (
	"fmt"
)

type AdvertiseStatusParams struct {
	Code         string
	LANState     string
	WANState     string
	MDNSState    string
	DHTState     string
	Reachability string
	NATState     string
	RelayAddrs   string
	PrivateAddrs string
	PublicAddrs  string
	PakePeers    []string
	PakeStates   map[string]string
}

func AdvertiseStatus(asp AdvertiseStatusParams, verbose bool) func() {
	// move up for info level log lines
	if verbose {
		fmt.Fprint(Out, fmt.Sprintf("Code:          %s\n", asp.Code))
		fmt.Fprint(Out, fmt.Sprintf("mDNS:          %s\n", asp.MDNSState))
		fmt.Fprint(Out, fmt.Sprintf("DHT:           %s\n", asp.DHTState))
		fmt.Fprint(Out, fmt.Sprintf("Network:       %s\n", asp.Reachability))
		fmt.Fprint(Out, fmt.Sprintf("NAT (udp/tcp): %s\n", asp.NATState))
		fmt.Fprint(Out, fmt.Sprintf("Multiaddresses\n"))
		fmt.Fprint(Out, fmt.Sprintf("   private:    %s\n", asp.PrivateAddrs))
		fmt.Fprint(Out, fmt.Sprintf("   public:     %s\n", asp.PublicAddrs))
		fmt.Fprint(Out, fmt.Sprintf("   relays:     %s\n", asp.RelayAddrs))
	}
	fmt.Fprint(Out, fmt.Sprintf("%s %s\t%s %s\n", Bold("Local Network:"), asp.LANState, Bold("Internet:"), asp.WANState))

	for _, p := range asp.PakePeers {
		fmt.Fprintf(Out, "  -> %s: %s\n", Bold(p), asp.PakeStates[p])
	}

	// return eraser function
	return func() {
		if verbose {
			fmt.Fprint(Out, "\033[9A")
		}
		if len(asp.PakePeers) > 0 {
			fmt.Fprintf(Out, "\033[%dA", len(asp.PakePeers))
		}
		fmt.Fprint(Out, "\033[1A\u001B[0J")
	}
}
