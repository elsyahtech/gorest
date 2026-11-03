package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
)

// ExpectListen marks the server as scheduled to start so Shutdown can wait for
// the listener to become ready or finish starting.
func (server *Server) ExpectListen() {
	if server == nil {
		return
	}

	server.lifecycleMu.Lock()
	server.listenScheduled = true
	server.lifecycleMu.Unlock()
}

// Listen starts Fiber and tracks its readiness and completion for shutdown.
func (server *Server) Listen(address string, config fiber.ListenConfig) error {
	if server == nil || server.SRV == nil {
		return errors.New("listen: server is not running")
	}

	server.ExpectListen()
	defer server.markListenDone()

	server.SRV.Hooks().OnListen(func(fiber.ListenData) error {
		server.markListening()
		return nil
	})

	return server.SRV.Listen(address, config) //nolint:wrapcheck
}

func (server *Server) markListenDone() {
	server.listenDoneOnce.Do(func() { close(server.listenDone) })
}

func (server *Server) markListening() {
	server.listeningOnce.Do(func() { close(server.listening) })
}

func (server *Server) setShutdownTimeout(timeout time.Duration) {
	server.lifecycleMu.Lock()
	server.shutdownTimeout = timeout
	server.lifecycleMu.Unlock()
}

// WaitForListening waits until the server listener is ready or exits.
func (server *Server) WaitForListening(ctx context.Context) error {
	if server == nil || server.SRV == nil {
		return errors.New("wait for server listener: server is not running")
	}

	if ctx == nil {
		return errors.New("wait for server listener: context is nil")
	}

	server.lifecycleMu.Lock()
	scheduled := server.listenScheduled
	listening := server.listening
	listenDone := server.listenDone
	server.lifecycleMu.Unlock()

	if !scheduled {
		return errors.New("wait for server listener: listener has not been scheduled")
	}

	select {
	case <-listening:
		return nil
	case <-listenDone:
		return errors.New("server listener stopped before becoming ready")
	case <-ctx.Done():
		return fmt.Errorf("wait for server listener: %w", ctx.Err())
	}
}

// Shutdown gracefully stops Fiber or managed prefork workers and waits for the
// listener lifecycle to finish.
func (server *Server) Shutdown(ctx context.Context) error {
	if server == nil || server.SRV == nil {
		return nil
	}

	if ctx == nil {
		return errors.New("shutdown: context is nil")
	}

	server.lifecycleMu.Lock()
	shutdownTimeout := server.shutdownTimeout
	scheduled := server.listenScheduled
	listening := server.listening
	listenDone := server.listenDone
	prefork := server.prefork
	server.lifecycleMu.Unlock()

	if shutdownTimeout <= 0 {
		shutdownTimeout = DefaultServerShutdownTimeout
	}

	shutdownContext, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	if prefork != nil {
		return prefork.shutdown(shutdownContext)
	}

	if !scheduled {
		return nil
	}

	return server.shutdownFiber(shutdownContext, listening, listenDone)
}

func (server *Server) shutdownFiber(
	shutdownContext context.Context,
	listening <-chan struct{},
	listenDone <-chan struct{},
) error {
	select {
	case <-listening:
	case <-listenDone:
		return nil
	case <-shutdownContext.Done():
		return fmt.Errorf("wait for server listener before shutdown: %w", shutdownContext.Err())
	}

	server.lifecycleMu.Lock()
	prefork := server.prefork
	server.lifecycleMu.Unlock()

	if prefork != nil {
		return prefork.shutdown(shutdownContext)
	}

	shutdownErr := server.SRV.ShutdownWithContext(shutdownContext)
	if shutdownErr != nil {
		select {
		case <-listenDone:
			return nil
		default:
		}

		return fmt.Errorf("shutdown Fiber server: %w", shutdownErr)
	}

	select {
	case <-listenDone:
		return nil
	case <-shutdownContext.Done():
		return fmt.Errorf("wait for Fiber listener to stop: %w", shutdownContext.Err())
	}
}
