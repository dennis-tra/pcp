package events

import (
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

type Emitter[T any] struct {
	closed atomic.Bool
	events chan T
}

func NewEmitter[T any]() *Emitter[T] {
	return &Emitter[T]{
		events: make(chan T),
	}
}

type Subscription[T any] struct {
	events <-chan T
}

func (e *Emitter[T]) Emit(evt T) {
	if e.closed.Load() {
		return
	}

	e.events <- evt
}

func (e *Emitter[T]) Subscribe() tea.Msg {
	val, more := <-e.events
	if !more {
		return nil
	}

	return Msg[T]{
		Value: val,
		rest:  e.events,
	}
}

func (e *Emitter[T]) Close() {
	e.closed.Store(true)
	// drain channel
	for {
		select {
		case <-e.events:
			continue
		default:
		}
		break
	}
	// close channel to indicate to all readers that we're done
	close(e.events)
}

type Msg[T any] struct {
	Value T
	rest  <-chan T
}

func (e Msg[T]) Listen() tea.Msg {
	val, more := <-e.rest
	if !more {
		return nil
	}

	return Msg[T]{
		Value: val,
		rest:  e.rest,
	}
}

func NewSubscription[T any](c <-chan T) Subscription[T] {
	return Subscription[T]{
		events: c,
	}
}

func (s Subscription[T]) Subscribe() tea.Msg {
	return Msg[T]{
		Value: <-s.events,
		rest:  s.events,
	}
}
