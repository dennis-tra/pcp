package term

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// Wait waits until the context was cancelled or we receive a SIGINT (e.g. ctrl+c) or SIGTERM (e.g. ctrl+d).
func Wait(ctx context.Context) chan bool {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigs:
		case <-ctx.Done():
		}

		done <- true
	}()

	return done
}
