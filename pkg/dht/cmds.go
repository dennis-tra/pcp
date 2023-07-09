package dht

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	tea "github.com/charmbracelet/bubbletea"
)

type (
	bootstrapResultMsg struct {
		err error
	}
	advertiseResult struct {
		offset time.Duration
		err    error
		fatal  bool
	}
	PeerMsg struct { // discoverResult
		Peer   peer.AddrInfo
		offset time.Duration
		err    error
		fatal  bool
	}
	stopMsg struct{ reason error }
)

func (d *DHT) Bootstrap() (*DHT, tea.Cmd) {
	if !(d.IsBootstrapped || d.State == StateBootstrapping) {
		d.State = StateBootstrapping
		return d, d.connectToBootstrapper
	} else {
		return d, nil
	}
}

func (d *DHT) Advertise(offsets ...time.Duration) (*DHT, tea.Cmd) {
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
	if reason != nil {
		d.State = StateError
		d.Err = reason
	} else {
		d.State = StateStopped
	}
	return d, nil
}
