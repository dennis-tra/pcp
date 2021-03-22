package service

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewService_lifecycle(t *testing.T) {
	s := New("test")

	s.ServiceStopped()
	err := s.ServiceStarted()
	require.NoError(t, err)
	s.ServiceStopped()
	s.ServiceStopped()
}

func TestNewService_shutdown(t *testing.T) {
	s := New("test")

	err := s.ServiceStarted()
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		s.Shutdown()
		wg.Done()
	}()
	go s.ServiceStopped()
	wg.Wait()
}

func TestNewService_contexts_stopped(t *testing.T) {
	s := New("test")
	err := s.ServiceStarted()
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			<-s.ServiceContext().Done()
			wg.Done()
		}()
	}
	go s.ServiceStopped()
	wg.Wait()
}

func TestNewService_contexts_shutdown(t *testing.T) {
	s := New("test")
	err := s.ServiceStarted()
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			<-s.ServiceContext().Done()
			wg.Done()
		}()
	}
	go s.Shutdown()
	wg.Wait()
}

func TestNewService_restart(t *testing.T) {
	s := New("test")
	err := s.ServiceStarted()
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			<-s.ServiceContext().Done()
			wg.Done()
		}()
	}
	wg.Add(1)
	go func() {
		s.Shutdown()
		wg.Done()
	}()
	go s.ServiceStopped()
	wg.Wait()

	err = s.ServiceStarted()
	require.Error(t, err)
	assert.Equal(t, ErrServiceAlreadyStarted, err)
}

func TestService_SigDone(t *testing.T) {
	s := New("test")
	err := s.ServiceStarted()
	require.NoError(t, err)
	s.ServiceStopped()
	<-s.SigDone()
}

func TestService_SigShutdown(t *testing.T) {
	s := New("test")
	err := s.ServiceStarted()
	require.NoError(t, err)
	go s.Shutdown()
	<-s.SigShutdown()
}
