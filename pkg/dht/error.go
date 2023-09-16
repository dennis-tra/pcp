package dht

import (
	"context"
	"errors"
)

type ErrConnThresholdNotReached struct {
	BootstrapErrs []error
}

func (e ErrConnThresholdNotReached) Error() string {
	return "could not establish enough connections to bootstrap peers"
}

func (e ErrConnThresholdNotReached) Log() {
	// If only one error is context.Canceled the user stopped the
	// program, and we don't want to print errors.
	for _, err := range e.BootstrapErrs {
		if errors.Is(err, context.Canceled) {
			return
		}
	}

	log.Warnln(e)
	for _, err := range e.BootstrapErrs {
		log.Warnf("\t%s\n", err)
	}

	log.Warnln("this means you will only be able to transfer files in your local network")
}
