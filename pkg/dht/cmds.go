package dht

import (
	"context"
	"errors"
	"fmt"
	"time"

	kaddht "github.com/libp2p/go-libp2p-kad-dht"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dennis-tra/pcp/pkg/config"
	"github.com/libp2p/go-libp2p/core/peer"
)

type (
	bootstrapResultMsg struct {
		err error
	}
	advertiseResultMsg struct {
		id    int
		err   error
		fatal bool
	}
	PeerMsg struct { // discoverResult
		Peer  peer.AddrInfo
		id    int
		Err   error
		fatal bool
	}
	stopMsg struct{ reason error }
)

func (d *DHT) Bootstrap() (*DHT, tea.Cmd) {
	if d.State >= StateBootstrapping && d.State != StateError {
		return d, nil
	}

	bootstrapPeers := config.Global.BoostrapAddrInfos()
	if len(bootstrapPeers) < ConnThreshold {
		d.reset()
		d.State = StateError
		d.Err = fmt.Errorf("too few Bootstrap peers configured (min %d)", ConnThreshold)
		return d, nil
	}

	var cmds []tea.Cmd
	for _, bp := range bootstrapPeers {
		d.BootstrapsPending += 1
		cmds = append(cmds, d.connectToBootstrapper(bp))
	}

	d.State = StateBootstrapping

	return d, tea.Batch(cmds...)
}

func (d *DHT) Advertise(offsets ...time.Duration) (*DHT, tea.Cmd) {
	var cmds []tea.Cmd

	if d.State == StateProviding || d.State == StateLookup {
		log.Fatal("DHT service already running")
		return d, nil
	}

	d.Err = nil

	kaddht.GetDefaultBootstrapPeerAddrInfos()
	for _, offset := range offsets {
		if _, found := d.services[offset]; found {
			continue
		}

		provideCtx, cancel := context.WithTimeout(d.ctx, provideTimeout)
		d.services[offset] = cancel
		cmds = append(cmds, d.provide(provideCtx, offset))
	}

	d.State = StateProviding

	return d, tea.Batch(cmds...)
}

func (d *DHT) Discover(offsets ...time.Duration) (*DHT, tea.Cmd) {
	var cmds []tea.Cmd

	if d.State == StateProviding || d.State == StateLookup {
		log.Fatal("DHT service already running")
		return d, nil
	}

	d.Err = nil

	for _, offset := range offsets {
		if _, found := d.services[offset]; found {
			continue
		}

		lookupCtx, cancel := context.WithCancel(d.ctx)
		d.services[offset] = cancel
		cmds = append(cmds, d.lookup(lookupCtx, offset))
	}

	d.State = StateLookup

	return d, tea.Batch(cmds...)
}

func (d *DHT) StopWithReason(reason error) (*DHT, tea.Cmd) {
	d.reset()
	if reason != nil && !errors.Is(reason, context.Canceled) {
		d.State = StateError
		d.Err = reason
	} else {
		d.State = StateStopped
	}

	return d, func() tea.Msg {
		if err := d.dht.Close(); err != nil {
			log.WithError(err).Warnln("Failed closing DHT")
		}
		return nil
	}
}
