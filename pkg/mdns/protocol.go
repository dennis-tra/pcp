package mdns

import (
	"sync"
	"time"

	"github.com/dennis-tra/pcp/internal/log"

	"github.com/libp2p/go-libp2p/core/host"

	"github.com/dennis-tra/pcp/internal/wrap"
	"github.com/dennis-tra/pcp/pkg/discovery"
	"github.com/dennis-tra/pcp/pkg/service"
)

type State string

const (
	StateIdle        State = "idle"
	StateAdvertising State = "advertising"
	StateError       State = "error"
	StateStopped     State = "stopped"
)

// These wrapped top level functions are here for testing purposes.
var (
	wraptime      wrap.Timer      = wrap.Time{}
	wrapdiscovery wrap.Discoverer = wrap.Discovery{}
)

// Timeout is the time until a new advertisement
// with a potentially new discovery ID is started.
var Timeout = time.Minute

// protocol encapsulates the logic for discovering peers
// via multicast DNS in the local network.
type protocol struct {
	host.Host
	service.Service

	did discovery.ID

	stateLk sync.RWMutex
	state   State
	err     error
}

func newProtocol(h host.Host) *protocol {
	return &protocol{
		Host:    h,
		Service: service.New("mDNS"),
		did:     discovery.ID{},
		state:   StateIdle,
	}
}

func (p *protocol) SetError(err error) {
	p.stateLk.Lock()
	p.state = StateError
	p.err = err
	p.stateLk.Unlock()
}

func (p *protocol) SetState(state State) {
	p.stateLk.Lock()
	p.state = state
	p.stateLk.Unlock()
}

func (p *protocol) State() State {
	p.stateLk.RLock()
	state := p.state
	log.Debugln("DHT State:", p.state)
	p.stateLk.RUnlock()

	return state
}

func (p *protocol) Error() error {
	p.stateLk.RLock()
	err := p.err
	p.stateLk.RUnlock()

	return err
}
