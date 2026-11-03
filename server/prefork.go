package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

const (
	preforkWorkerEnv         = "GOREST_PREFORK_WORKER"
	preforkControlAddrEnv    = "GOREST_PREFORK_CONTROL_ADDR"
	preforkControlTokenEnv   = "GOREST_PREFORK_CONTROL_TOKEN"
	preforkReadyTimeout      = 30 * time.Second
	preforkRegistrationLimit = 256
)

type preforkSupervisor struct {
	workerContext context.Context
	result        error
	control       net.Listener
	ready         chan preforkWorkerReady
	stopRequested chan context.Context
	cancelWorkers context.CancelFunc
	workers       map[int]*preforkWorker
	exits         chan preforkWorkerExit
	server        *Server
	failures      chan error
	done          chan struct{}
	token         string
	config        ListenerConfig
	workerCount   int
	readyCount    int
	recoveries    int
	stopOnce      sync.Once
	resultMu      sync.Mutex
	stopping      bool
}

type preforkWorker struct {
	control net.Conn
	command *exec.Cmd
	ready   bool
}

type preforkWorkerExit struct {
	err error
	pid int
}

type preforkWorkerReady struct {
	control net.Conn
	token   string
	pid     int
}

func newPreforkSupervisor(server *Server, config ListenerConfig) (*preforkSupervisor, error) {
	control, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for prefork worker registration: %w", err)
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		closePreforkResource(control)
		return nil, fmt.Errorf("create prefork registration token: %w", err)
	}

	workerContext, cancelWorkers := context.WithCancel(context.Background())

	workerCount := runtime.GOMAXPROCS(0)
	if workerCount < 1 {
		workerCount = 1
	}

	return &preforkSupervisor{
		server:        server,
		config:        config,
		control:       control,
		token:         hex.EncodeToString(tokenBytes),
		workerCount:   workerCount,
		workers:       make(map[int]*preforkWorker, workerCount),
		exits:         make(chan preforkWorkerExit, workerCount+1),
		ready:         make(chan preforkWorkerReady, workerCount+1),
		failures:      make(chan error, 1),
		stopRequested: make(chan context.Context, 1),
		done:          make(chan struct{}),
		workerContext: workerContext,
		cancelWorkers: cancelWorkers,
	}, nil
}

func isManagedPreforkWorker() bool {
	return os.Getenv(preforkWorkerEnv) == "1" &&
		os.Getenv("FASTHTTP_PREFORK_CHILD") == "1" &&
		os.Getenv(preforkControlAddrEnv) != "" &&
		os.Getenv(preforkControlTokenEnv) != ""
}

func (server *Server) setPreforkSupervisor(supervisor *preforkSupervisor) {
	server.lifecycleMu.Lock()
	server.prefork = supervisor
	server.lifecycleMu.Unlock()
}

func (supervisor *preforkSupervisor) run() (runErr error) {
	supervisor.server.ExpectListen()
	defer func() {
		closePreforkResource(supervisor.control)
		supervisor.cancelWorkers()
		supervisor.setResult(runErr)
		close(supervisor.done)
	}()

	go supervisor.acceptWorkers()

	for range supervisor.workerCount {
		if err := supervisor.startWorker(); err != nil {
			supervisor.stopWorkers(context.Background())
			return err
		}
	}

	if err := supervisor.awaitInitialReadiness(); err != nil {
		if errors.Is(err, errPreforkShutdownRequested) {
			return nil
		}

		supervisor.stopWorkers(context.Background())

		return err
	}

	supervisor.server.markListening()

	if err := supervisor.superviseWorkers(); err != nil {
		supervisor.stopWorkers(context.Background())
		return err
	}

	return nil
}

func (supervisor *preforkSupervisor) awaitInitialReadiness() error {
	startupDeadline := time.NewTimer(preforkReadyTimeout)
	defer startupDeadline.Stop()

	for supervisor.readyCount < supervisor.workerCount {
		select {
		case worker := <-supervisor.ready:
			if err := supervisor.registerReadyWorker(worker); err != nil {
				return err
			}
		case exit := <-supervisor.exits:
			if err := supervisor.recoverWorker(exit); err != nil {
				return err
			}
		case err := <-supervisor.failures:
			return err
		case ctx := <-supervisor.stopRequested:
			supervisor.stopWorkers(ctx)
			return errPreforkShutdownRequested
		case <-startupDeadline.C:
			return fmt.Errorf("prefork workers did not become ready within %s", preforkReadyTimeout)
		}
	}

	return nil
}

func (supervisor *preforkSupervisor) superviseWorkers() error {
	for {
		select {
		case ctx := <-supervisor.stopRequested:
			supervisor.stopWorkers(ctx)
			return nil
		case worker := <-supervisor.ready:
			if err := supervisor.registerReadyWorker(worker); err != nil {
				return err
			}
		case exit := <-supervisor.exits:
			if err := supervisor.recoverWorker(exit); err != nil {
				return ignorePreforkShutdown(err)
			}
		case err := <-supervisor.failures:
			return err
		}
	}
}

func ignorePreforkShutdown(err error) error {
	if errors.Is(err, errPreforkShutdownRequested) {
		return nil
	}

	return err
}

func (supervisor *preforkSupervisor) recoverWorker(exit preforkWorkerExit) error {
	restart, err := supervisor.handleWorkerExit(exit)
	if err != nil || !restart {
		return err
	}

	ctx, err := supervisor.waitBeforeRecovery()
	if err != nil {
		if errors.Is(err, errPreforkShutdownRequested) {
			supervisor.stopWorkers(ctx)
			return err
		}

		return err
	}

	return supervisor.startWorker()
}
