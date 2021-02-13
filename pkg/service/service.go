package service

import (
	"context"
	"sync"

	"github.com/pkg/errors"
)

type ServiceState uint8

const (
	Unstarted ServiceState = iota
	Started
	Stopping
	Stopped
)

var ErrServiceAlreadyStarted = errors.New("the service was already started in the past")

type Service struct {
	// A context that can be used for long running
	// io operations of the service. This context
	// gets cancelled when the service receives a
	// shutdown signal.
	ctx    context.Context
	cancel context.CancelFunc

	lk    sync.RWMutex
	state ServiceState

	// When message is sent to this channel it
	// starts to gracefully shut down.
	shutdown chan struct{}

	// When a message is sent to this channel
	// the service is shut down.
	done chan struct{}
}

func New() *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		ctx:      ctx,
		cancel:   cancel,
		state:    Unstarted,
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (s *Service) ServiceStarted() error {
	s.lk.Lock()
	defer s.lk.Unlock()

	if s.state != Unstarted {
		return ErrServiceAlreadyStarted
	}
	s.state = Started

	go func() {
		select {
		case <-s.shutdown:
		case <-s.done:
		}
		s.cancel()
	}()

	return nil
}

func (s *Service) SigShutdown() chan struct{} {
	return s.shutdown
}

func (s *Service) SigDone() chan struct{} {
	return s.done
}

func (s *Service) ServiceStopped() {
	s.lk.Lock()
	defer s.lk.Unlock()

	if s.state == Unstarted || s.state == Stopped {
		return
	}
	s.state = Stopped

	close(s.done)
}

func (s *Service) ServiceContext() context.Context {
	return s.ctx
}

func (s *Service) Shutdown() {
	s.lk.Lock()
	if s.state != Started {
		s.lk.Unlock()
		return
	}
	s.state = Stopping
	s.lk.Unlock()

	close(s.shutdown)
	<-s.done
}
