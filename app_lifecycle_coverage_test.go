package gorest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/server"
)

func newLifecycleTestLogger(t *testing.T) *log.Log {
	t.Helper()
	logger, message, err := log.New(&log.Config{
		LogDir:  t.TempDir(),
		LogFile: "lifecycle.log",
	}, "lifecycle-test")
	if err != nil {
		t.Fatalf("log.New() error = %v, message = %q", err, message)
	}
	t.Cleanup(func() {
		_, _, _ = logger.Close()
	})
	return logger
}

func TestHTTPServerDisabledDecision(t *testing.T) {
	httpServer, _, err := server.New(nil)
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	config := &server.Config{}
	tests := []struct {
		name   string
		config Config
		server *server.Server
		want   bool
	}{
		{name: "debug mode", config: Config{Debug: true, server: config}, server: httpServer, want: true},
		{name: "missing runtime server", config: Config{server: config}, want: true},
		{name: "missing server config", config: Config{}, server: httpServer, want: true},
		{name: "server enabled", config: Config{server: config}, server: httpServer},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := &App{config: test.config, server: test.server}
			if got := app.httpServerDisabled(); got != test.want {
				t.Fatalf("httpServerDisabled() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestForwardShutdownSignalCancelsStartupAndStopsWhenAlreadyCanceled(t *testing.T) {
	startupCtx, cancelStartup := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	shutdownSignal := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		forwardShutdownSignal(startupCtx, quit, cancelStartup, shutdownSignal)
	}()

	quit <- os.Interrupt
	select {
	case got := <-shutdownSignal:
		if got != os.Interrupt {
			t.Fatalf("shutdown signal = %v, want %v", got, os.Interrupt)
		}
	case <-time.After(time.Second):
		t.Fatal("forwardShutdownSignal() did not forward the received signal")
	}
	select {
	case <-startupCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("forwardShutdownSignal() did not cancel startup")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("forwardShutdownSignal() did not return after cancellation")
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	quit = make(chan os.Signal)
	shutdownSignal = make(chan os.Signal, 1)
	forwardShutdownSignal(canceledCtx, quit, cancel, shutdownSignal)
	select {
	case got := <-shutdownSignal:
		t.Fatalf("already-canceled startup unexpectedly forwarded %v", got)
	default:
	}
}

func TestRunStartupWaitsForShutdownWhenHTTPServerIsDisabled(t *testing.T) {
	app := &App{
		log:      newLifecycleTestLogger(t),
		config:   Config{Debug: true},
		services: &service{},
	}
	shutdownSignal := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.runStartup(context.Background(), shutdownSignal)
	}()

	shutdownSignal <- os.Interrupt
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runStartup() did not shut down after receiving a signal")
	}
}

func TestRunStartupAndWaitForShutdownHandleCanceledContext(t *testing.T) {
	app := &App{
		log:      newLifecycleTestLogger(t),
		config:   Config{Debug: true},
		services: &service{},
	}
	startupCtx, cancel := context.WithCancel(context.Background())
	cancel()
	shutdownSignal := make(chan os.Signal, 1)
	shutdownSignal <- os.Interrupt

	app.runStartup(startupCtx, shutdownSignal)

	startupCtx, cancel = context.WithCancel(context.Background())
	cancel()
	shutdownSignal = make(chan os.Signal, 1)
	shutdownSignal <- os.Interrupt
	waitApp := &App{log: newLifecycleTestLogger(t)}
	waitApp.waitForShutdown(startupCtx, shutdownSignal)
}

func TestAppStartAndProductionWarnings(t *testing.T) {
	startLogDir := t.TempDir()
	startLogger, message, err := log.New(&log.Config{LogDir: startLogDir, LogFile: "lifecycle.log"}, "lifecycle-test")
	if err != nil {
		t.Fatalf("log.New() error = %v, message = %q", err, message)
	}
	startApp := &App{
		log:      startLogger,
		config:   Config{Debug: true, Environment: Testing},
		services: &service{},
	}
	startDone := make(chan struct{})
	go func() {
		defer close(startDone)
		startApp.Start()
	}()

	deadline := time.Now().Add(3 * time.Second)
	readyLogged := false
	for time.Now().Before(deadline) {
		logOutput, err := os.ReadFile(filepath.Join(startLogDir, "lifecycle.log"))
		if err == nil && strings.Contains(string(logOutput), "gorest is ready; HTTP server is disabled") {
			readyLogged = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !readyLogged {
		t.Fatal("app.Start() did not reach the ready state")
	}
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find current process: %v", err)
	}
	if err := process.Signal(os.Interrupt); err != nil {
		t.Fatalf("send lifecycle shutdown signal: %v", err)
	}
	select {
	case <-startDone:
	case <-time.After(3 * time.Second):
		t.Fatal("app.Start() did not return after receiving the shutdown signal")
	}

	logDir := t.TempDir()
	logger, message, err := log.New(&log.Config{LogDir: logDir, LogFile: "lifecycle.log"}, "lifecycle-test")
	if err != nil {
		t.Fatalf("log.New() error = %v, message = %q", err, message)
	}
	app := &App{
		log: logger,
		config: Config{
			Environment: Production,
			database:    &database.Config{Driver: database.POSTGRES, TLS: database.TLSDisabled},
		},
	}
	app.logProductionWarnings()
	if _, _, err := logger.Close(); err != nil {
		t.Fatalf("close warning logger: %v", err)
	}

	logOutput, err := os.ReadFile(filepath.Join(logDir, "lifecycle.log"))
	if err != nil {
		t.Fatalf("read production warning log: %v", err)
	}
	if !strings.Contains(string(logOutput), "PostgreSQL TLS is disabled") {
		t.Fatalf("production warnings did not include database TLS warning: %s", logOutput)
	}
}

func TestStartHTTPServerAndGracefulSignalShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve HTTP port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release HTTP port: %v", err)
	}

	serverConfig := server.ConfigDefault
	serverConfig.Host = "127.0.0.1"
	serverConfig.Port = port
	httpServer, _, err := server.New(&serverConfig)
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	app := &App{
		log:      newLifecycleTestLogger(t),
		config:   Config{AppName: "lifecycle-test", Version: "test", Timezone: "UTC", Environment: Testing, server: &serverConfig},
		server:   httpServer,
		services: &service{},
	}

	shutdownSignal := make(chan os.Signal, 1)
	startDone := make(chan struct{})
	httpServer.ExpectListen()
	go func() {
		defer close(startDone)
		app.startHTTPServer(context.Background(), shutdownSignal)
	}()

	readyCtx, cancelReady := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelReady()
	if err := httpServer.WaitForListening(readyCtx); err != nil {
		t.Fatalf("HTTP server did not become ready: %v", err)
	}

	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("request default route: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Errorf("close default route response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("default route status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	shutdownSignal <- os.Interrupt
	select {
	case <-startDone:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP server lifecycle did not finish after shutdown signal")
	}
}

func TestWaitForListenerReadyAndUnexpectedServerStop(t *testing.T) {
	app := &App{log: newLifecycleTestLogger(t)}
	listenerReady := make(chan error, 1)
	listenerReady <- nil
	if !app.waitForListenerReady(
		context.Background(),
		make(chan os.Signal),
		make(chan error),
		listenerReady,
	) {
		t.Fatal("waitForListenerReady() did not accept a ready listener")
	}

	serverDone := make(chan error, 1)
	serverDone <- nil
	app.waitForHTTPServerShutdown(make(chan os.Signal), serverDone)

	signal := make(chan os.Signal, 1)
	signal <- os.Interrupt
	if app.waitForListenerReady(
		context.Background(),
		signal,
		make(chan error),
		make(chan error),
	) {
		t.Fatal("waitForListenerReady() accepted a shutdown signal as readiness")
	}
}

func TestWaitForListenerReadyHonorsStartupCancellation(t *testing.T) {
	logDir := t.TempDir()
	logger, message, err := log.New(&log.Config{
		LogDir:  logDir,
		LogFile: "lifecycle.log",
	}, "lifecycle-test")
	if err != nil {
		t.Fatalf("log.New() error = %v, message = %q", err, message)
	}

	app := &App{log: logger}
	startupCtx, cancel := context.WithCancel(context.Background())
	cancel()
	shutdownSignal := make(chan os.Signal, 1)
	shutdownSignal <- os.Interrupt

	if app.waitForListenerReady(
		startupCtx,
		shutdownSignal,
		make(chan error),
		make(chan error),
	) {
		t.Fatal("waitForListenerReady() accepted a canceled startup context")
	}

	select {
	case sig := <-shutdownSignal:
		t.Fatalf("shutdown signal %v was not handled", sig)
	default:
	}

	logOutput, err := os.ReadFile(filepath.Join(logDir, "lifecycle.log"))
	if err != nil {
		t.Fatalf("read lifecycle log: %v", err)
	}
	if !strings.Contains(string(logOutput), "Application received shutdown signal") {
		t.Fatalf("shutdown cleanup was not logged: %s", logOutput)
	}
}

func TestWaitForListenerReadyNilContext(t *testing.T) {
	app := &App{log: newLifecycleTestLogger(t)}
	listenerReady := make(chan error, 1)
	listenerReady <- nil
	if !app.waitForListenerReady(nil, make(chan os.Signal), make(chan error), listenerReady) {
		t.Fatal("waitForListenerReady() rejected a nil startup context when listener became ready")
	}
}

func TestHandleListenerReadyErrorAfterCancellation(t *testing.T) {
	app := &App{log: newLifecycleTestLogger(t)}
	startupCtx, cancel := context.WithCancel(context.Background())
	cancel()
	shutdownSignal := make(chan os.Signal, 1)
	shutdownSignal <- os.Interrupt

	app.handleListenerReadyError(startupCtx, shutdownSignal, context.DeadlineExceeded)
}

func TestLogProductionWarningsSkipsNonProductionConfiguration(t *testing.T) {
	app := &App{
		log:    newLifecycleTestLogger(t),
		config: Config{Environment: Development},
	}

	app.logProductionWarnings()
}
