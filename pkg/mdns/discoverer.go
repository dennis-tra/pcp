package mdns

import (
	"net"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/pkg/errors"
	"github.com/whyrusleeping/mdns"

	"github.com/dennis-tra/pcp/internal/log"
	pcpnode "github.com/dennis-tra/pcp/pkg/node"
)

type Discoverer struct {
	*protocol
	code string
}

func NewDiscoverer(node *pcpnode.Node, code string) *Discoverer {
	return &Discoverer{protocol: newProtocol(node), code: code}
}

func (d *Discoverer) Discover(handler pcpnode.PeerHandler) error {
	if err := d.ServiceStarted(); err != nil {
		return err
	}
	defer d.ServiceStopped()

	for {
		entriesCh := make(chan *mdns.ServiceEntry, 16)
		go d.drainEntriesChan(entriesCh, handler)

		qp := &mdns.QueryParam{
			Domain:  "local",
			Entries: entriesCh,
			Service: d.code, // keep in sync with advertiser
			Timeout: time.Second * 5,
		}

		err := mdns.Query(qp)
		if err != nil {
			log.Warningln("mdns lookup error", err)
		}
		close(entriesCh)

		select {
		case <-d.SigShutdown():
			return nil
		default:
		}
	}
}

func (d *Discoverer) Shutdown() {
	d.Service.Shutdown()
}

func (d *Discoverer) drainEntriesChan(entries chan *mdns.ServiceEntry, handler pcpnode.PeerHandler) {
	for entry := range entries {

		pi, err := parseServiceEntry(entry)
		if err != nil {
			continue
		}

		if pi.ID == d.ID() {
			continue
		}

		pi.Addrs = onlyPrivate(pi.Addrs)
		if !isRoutable(pi) {
			continue
		}

		go handler.HandlePeer(pi)
	}
}

func parseServiceEntry(entry *mdns.ServiceEntry) (peer.AddrInfo, error) {
	p, err := peer.Decode(entry.Info)
	if err != nil {
		return peer.AddrInfo{}, errors.Wrap(err, "error parsing peer ID from mdns entry")
	}

	var addr net.IP
	if entry.AddrV4 != nil {
		addr = entry.AddrV4
	} else if entry.AddrV6 != nil {
		addr = entry.AddrV6
	} else {
		return peer.AddrInfo{}, errors.Wrap(err, "error parsing multiaddr from mdns entry: no IP address found")
	}

	maddr, err := manet.FromNetAddr(&net.TCPAddr{IP: addr, Port: entry.Port})
	if err != nil {
		return peer.AddrInfo{}, errors.Wrap(err, "error parsing multiaddr from mdns entry")
	}

	return peer.AddrInfo{
		ID:    p,
		Addrs: []ma.Multiaddr{maddr},
	}, nil
}

func isRoutable(pi peer.AddrInfo) bool {
	return len(pi.Addrs) > 0
}

// Filter out addresses that are public - only allow private ones.
func onlyPrivate(addrs []ma.Multiaddr) []ma.Multiaddr {
	routable := []ma.Multiaddr{}
	for _, addr := range addrs {
		if manet.IsPrivateAddr(addr) {
			routable = append(routable, addr)
		}
	}
	return routable
}
