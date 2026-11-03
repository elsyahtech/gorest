package server

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var errPreforkShutdownRequested = errors.New("prefork shutdown requested")

func (supervisor *preforkSupervisor) stopWorkers(ctx context.Context) {
	supervisor.stopping = true
	closePreforkResource(supervisor.control)

	var contextDone <-chan struct{}
	if ctx != nil {
		contextDone = ctx.Done()
	}

	supervisor.shutdownSignals()

	grace := supervisor.config.PreforkShutdownGracePeriod
	if grace <= 0 {
		grace = 5 * time.Second
	}

	timer := time.NewTimer(grace)
	defer timer.Stop()

	for len(supervisor.workers) > 0 {
		select {
		case exit := <-supervisor.exits:
			supervisor.removeWorker(exit.pid)
		case <-timer.C:
			supervisor.killWorkers()
			timer.Reset(5 * time.Second)
		case <-contextDone:
			supervisor.killWorkers()
			timer.Reset(5 * time.Second)

			contextDone = nil
		}
	}
}

func (supervisor *preforkSupervisor) shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("shutdown prefork supervisor: context is nil")
	}

	supervisor.stopOnce.Do(func() {
		supervisor.stopRequested <- ctx
	})

	select {
	case <-supervisor.done:
		supervisor.resultMu.Lock()
		defer supervisor.resultMu.Unlock()

		return supervisor.result
	case <-ctx.Done():
		return fmt.Errorf("shutdown prefork supervisor: %w", ctx.Err())
	}
}

func (supervisor *preforkSupervisor) setResult(err error) {
	supervisor.resultMu.Lock()
	supervisor.result = err
	supervisor.resultMu.Unlock()
}
