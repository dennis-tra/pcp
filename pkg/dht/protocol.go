package dht

import (
	"fmt"
	"sync"

	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/service"
)

// These wrapped top level functions are here for testing purposes.
var (
	wrapDHT   wrap.DHTer   = wrap.DHT{}
	wraptime  wrap.Timer   = wrap.Time{}
	wrapmanet wrap.Maneter = wrap.Manet{}
)

// protocol encapsulates the logic for discovering peers
// through providing it in the IPFS DHT.
type protocol struct {
	host.Host

	// Service holds an abstraction of a long-running
	// service that is started and stopped externally.
	service.Service
	dht wrap.IpfsDHT
	did discovery.ID

	stateLk sync.RWMutex
	state   *State
}

func newProtocol(h host.Host, dht wrap.IpfsDHT) *protocol {
	p := &protocol{
		Host:    h,
		dht:     dht,
		Service: service.New("DHT"),
		did:     discovery.ID{},
		state: &State{
			Stage:        StageIdle,
			Reachability: network.ReachabilityUnknown,
			NATTypeTCP:   network.NATDeviceTypeUnknown,
			NATTypeUDP:   network.NATDeviceTypeUnknown,
		},
	}

	// populate the address slices
	p.state.populateAddrs(h.Addrs())

	return p
}

func (p *protocol) setError(err error) {
	p.stateLk.Lock()
	p.state.Stage = StageError
	p.state.Err = err
	p.stateLk.Unlock()
}

func (p *protocol) setState(fn func(state *State)) {
	p.stateLk.Lock()
	fn(p.state)
	log.Debugln("DHT State:", p.state)
	p.stateLk.Unlock()
}

func (p *protocol) setStage(stage Stage) {
	p.setState(func(s *State) { s.Stage = stage })
}

func (p *protocol) State() State {
	p.stateLk.RLock()
	state := p.state
	p.stateLk.RUnlock()

	return *state
}

// bootstrap connects to a set of bootstrap nodes to connect
// to the DHT.
func (p *protocol) bootstrap() error {
	p.setState(func(s *State) { s.Stage = StageBootstrapping })

	peers := kaddht.GetDefaultBootstrapPeerAddrInfos()
	peerCount := len(peers)
	if peerCount == 0 {
		return fmt.Errorf("no bootstrap peers configured")
	}

	// Asynchronously connect to all bootstrap peers and send
	// potential errors to a channel. This channel is used
	// to capture the errors and check if we have established
	// enough connections. An error group (errgroup) cannot
	// be used here as it exits as soon as an error is thrown
	// in one of the Go-Routines.
	var wg sync.WaitGroup
	errChan := make(chan error, peerCount)
	for _, bp := range peers {
		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			errChan <- p.Connect(p.ServiceContext(), pi)
		}(bp)
	}

	// Close error channel after all connection attempts are done
	// to signal the for-loop below to stop.
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Reading the error channel and collect errors.
	errs := ErrConnThresholdNotReached{BootstrapErrs: []error{}}
	for {
		err, ok := <-errChan
		if !ok {
			// channel was closed.
			break
		} else if err != nil {
			errs.BootstrapErrs = append(errs.BootstrapErrs, err)
		}
	}

	// If we could not establish enough connections return an error
	if peerCount-len(errs.BootstrapErrs) < ConnThreshold {
		return errs
	}

	return nil
}

// checkNetwork subscribes to a couple of libp2p events that fire after certain network conditions where determined.
func (p *protocol) checkNetwork() error {
	p.setState(func(s *State) { s.Stage = StageAnalyzingNetwork })

	evtTypes := []interface{}{
		new(event.EvtLocalReachabilityChanged),
		new(event.EvtNATDeviceTypeChanged),
		new(event.EvtLocalAddressesUpdated),
	}
	sub, err := p.EventBus().Subscribe(evtTypes)
	if err != nil {
		return fmt.Errorf("subscribe to libp2p eventbus: %w", err)
	}
	defer sub.Close()

	for {
		var e interface{}

		select {
		case <-p.ServiceContext().Done():
			return p.ServiceContext().Err()
		case e = <-sub.Out():
		}

		p.stateLk.Lock()
		switch evt := e.(type) {
		case event.EvtLocalReachabilityChanged:
			p.state.Reachability = evt.Reachability
		case event.EvtNATDeviceTypeChanged:
			switch evt.TransportProtocol {
			case network.NATTransportUDP:
				p.state.NATTypeUDP = evt.NatDeviceType
			case network.NATTransportTCP:
				p.state.NATTypeTCP = evt.NatDeviceType
			}
		case event.EvtLocalAddressesUpdated:
			maddrs := make([]ma.Multiaddr, len(evt.Current))
			for i, update := range evt.Current {
				maddrs[i] = update.Address
			}
			p.state.populateAddrs(maddrs)
		}
		p.stateLk.Unlock()

		if p.state.Reachability == network.ReachabilityPrivate && p.state.NATTypeUDP == network.NATDeviceTypeSymmetric && p.state.NATTypeTCP == network.NATDeviceTypeSymmetric {
			return fmt.Errorf("private network with symmetric NAT")
		}

		// we have public reachability, we're good to go with the DHT
		if p.state.Reachability == network.ReachabilityPublic && len(p.state.PublicAddrs) > 0 {
			return nil
		}

		// we are in a private network, but have at least one cone NAT and at least one relay address
		if p.state.Reachability == network.ReachabilityPrivate && (p.state.NATTypeUDP == network.NATDeviceTypeCone || p.state.NATTypeTCP == network.NATDeviceTypeCone) && len(p.state.RelayAddrs) > 0 {
			return nil
		}
	}
}
