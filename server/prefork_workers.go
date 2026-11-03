package server

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func (supervisor *preforkSupervisor) startWorker() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve application executable for prefork worker: %w", err)
	}

	command := exec.CommandContext(supervisor.workerContext, executable, os.Args[1:]...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	command.Env = setEnvironment(os.Environ(), map[string]string{
		"FASTHTTP_PREFORK_CHILD": "1",
		preforkWorkerEnv:         "1",
		preforkControlAddrEnv:    supervisor.control.Addr().String(),
		preforkControlTokenEnv:   supervisor.token,
	})
	if err := command.Start(); err != nil {
		return fmt.Errorf("start prefork worker: %w", err)
	}

	pid := command.Process.Pid

	supervisor.workers[pid] = &preforkWorker{command: command}
	go func() {
		supervisor.exits <- preforkWorkerExit{pid: pid, err: command.Wait()}
	}()

	return nil
}

func (supervisor *preforkSupervisor) handleWorkerExit(exit preforkWorkerExit) (bool, error) {
	if !supervisor.removeWorker(exit.pid) {
		return false, nil
	}

	if supervisor.stopping {
		return false, nil
	}

	supervisor.recoveries++
	stdlog.Printf("gorest prefork: worker %d exited (%v); restart %d of %d",
		exit.pid, exit.err, supervisor.recoveries, supervisor.config.PreforkRecoverThreshold)

	if supervisor.recoveries > supervisor.config.PreforkRecoverThreshold {
		return false, fmt.Errorf(
			"prefork worker recovery threshold exceeded: %d exits, threshold %d",
			supervisor.recoveries,
			supervisor.config.PreforkRecoverThreshold,
		)
	}

	return true, nil
}

func (supervisor *preforkSupervisor) removeWorker(pid int) bool {
	worker, exists := supervisor.workers[pid]
	if !exists {
		return false
	}

	if worker.ready {
		supervisor.readyCount--
	}

	if worker.control != nil {
		closePreforkResource(worker.control)
	}

	delete(supervisor.workers, pid)

	return true
}

func (supervisor *preforkSupervisor) waitBeforeRecovery() (context.Context, error) {
	if supervisor.config.PreforkRecoverInterval == 0 {
		select {
		case ctx := <-supervisor.stopRequested:
			return ctx, errPreforkShutdownRequested
		default:
			return nil, nil
		}
	}

	timer := time.NewTimer(supervisor.config.PreforkRecoverInterval)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil, nil
	case ctx := <-supervisor.stopRequested:
		return ctx, errPreforkShutdownRequested
	}
}

func (supervisor *preforkSupervisor) shutdownSignals() {
	for _, worker := range supervisor.workers {
		if err := worker.command.Process.Signal(syscall.SIGTERM); err != nil &&
			!errors.Is(err, os.ErrProcessDone) {
			stdlog.Printf("gorest prefork: send SIGTERM to worker %d: %v", worker.command.Process.Pid, err)
		}
	}
}

func (supervisor *preforkSupervisor) killWorkers() {
	for _, worker := range supervisor.workers {
		if err := worker.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			stdlog.Printf("gorest prefork: kill worker %d: %v", worker.command.Process.Pid, err)
		}
	}
}
