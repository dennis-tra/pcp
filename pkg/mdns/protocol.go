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
	state   *State
}

func newProtocol(h host.Host) *protocol {
	return &protocol{
		Host:    h,
		Service: service.New("mDNS"),
		did:     discovery.ID{},
		state: &State{
			Stage: StageIdle,
			Err:   nil,
		},
	}
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
	log.Debugln("mDNS State:", p.state)
	p.stateLk.Unlock()
}

func (p *protocol) setStage(stage Stage) {
	p.setState(func(s *State) { s.Stage = stage })
}

func (p *protocol) State() State {
	p.stateLk.RLock()
	state := *p.state
	p.stateLk.RUnlock()

	return state
}
