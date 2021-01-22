package send

import (
	"fmt"
	"github.com/dennis-tra/pcp/pkg/commons"
	"net"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/whyrusleeping/mdns"
)

// QueryPeers will send DNS multicast messages in the local network to
// find all peers waiting to receive files.
func QueryPeers() ([]*mdns.ServiceEntry, error) {

	// TODO: Change this to an unbuffered channel. This is currently not
	// possible because the mdns library does an unblocking send into
	// their channel and therefore only one entry would be written
	// and subsequent sends will be dropped, because we don't get
	// the chance to drown the channel. Adjust the library:
	// https://github.com/dennis-tra/pcp/projects/1#card-53284716
	entriesCh := make(chan *mdns.ServiceEntry, 16)
	query := &mdns.QueryParam{
		Service:             commons.ServiceTag,
		Domain:              "local",
		Timeout:             time.Second,
		Entries:             entriesCh,
		WantUnicastResponse: true,
	}

	err := mdns.Query(query)
	if err != nil {
		return nil, err
	}
	close(entriesCh)

	services := []*mdns.ServiceEntry{}
	for entry := range entriesCh {
		if _, err := peer.Decode(entry.Info); err != nil {
			continue
		}
		services = append(services, entry)
	}

	return services, nil
}

// parseServiceEntry takes an mDNS service entry and extracts information
// to create an address info struct of our peer.
func parseServiceEntry(entry *mdns.ServiceEntry) (peer.AddrInfo, error) {

	var addrInfo peer.AddrInfo

	pId, err := peer.Decode(entry.Info)
	if err != nil {
		return addrInfo, fmt.Errorf("error parsing peer ID from mdns entry: %w", err)
	}

	var addr net.IP
	if entry.AddrV4 != nil {
		addr = entry.AddrV4
	} else if entry.AddrV6 != nil {
		addr = entry.AddrV6
	} else {
		return addrInfo, fmt.Errorf("error parsing multiaddr from mdns entry: no IP address found")
	}

	maddr, err := manet.FromNetAddr(&net.TCPAddr{IP: addr, Port: entry.Port})
	if err != nil {
		return addrInfo, fmt.Errorf("error parsing multiaddr from mdns entry: %w", err)
	}

	addrInfo = peer.AddrInfo{
		ID:    pId,
		Addrs: []ma.Multiaddr{maddr},
	}

	return addrInfo, nil
}
