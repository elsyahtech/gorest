//go:build integration

package app_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/server"
)

const (
	appHelperEnv      = "GOREST_IT_APP_HELPER"
	appPortEnv        = "GOREST_IT_APP_PORT"
	appStartedFileEnv = "GOREST_IT_APP_STARTED_FILE"
)

func TestGORESTAppStartAndSignalShutdown(t *testing.T) {
	if os.Getenv(appHelperEnv) == "1" {
		runAppHelper(t)
		return
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve HTTP port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release HTTP port: %v", err)
	}

	workDir := t.TempDir()
	startedFile := filepath.Join(workDir, "request-started")
	command := exec.CommandContext(
		testContext(t, 20*time.Second),
		os.Args[0],
		"-test.run=^TestGORESTAppStartAndSignalShutdown$",
	)
	command.Dir = workDir
	command.Env = append(os.Environ(),
		appHelperEnv+"=1",
		appPortEnv+"="+strconv.Itoa(port),
		appStartedFileEnv+"="+startedFile,
	)
	var processOutput bytes.Buffer
	command.Stdout = &processOutput
	command.Stderr = &processOutput
	if err := command.Start(); err != nil {
		t.Fatalf("start GOREST app helper: %v", err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
		}
	})

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHTTP(t, baseURL+"/ready")
	waitForHTTP(t, baseURL+"/gorest/live")

	responseResult := make(chan error, 1)
	go func() {
		response, err := http.Get(baseURL + "/slow")
		if err != nil {
			responseResult <- err
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			responseResult <- fmt.Errorf("slow request status = %d, want %d", response.StatusCode, http.StatusOK)
			return
		}
		responseResult <- nil
	}()
	waitForFile(t, startedFile)

	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM to GOREST app: %v\n%s", err, processOutput.String())
	}

	select {
	case err := <-responseResult:
		if err != nil {
			t.Fatalf("in-flight request did not finish during graceful shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight request did not complete during shutdown")
	}

	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()
	select {
	case err := <-waitResult:
		if err != nil {
			t.Fatalf("GOREST app helper exited with error: %v\n%s", err, processOutput.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("GOREST app helper did not exit after SIGTERM\n%s", processOutput.String())
	}

	logPath := filepath.Join(workDir, "logs", "app.log")
	logOutput, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read app lifecycle log %q: %v", logPath, err)
	}
	if !strings.Contains(string(logOutput), "Application received shutdown signal") {
		t.Error("app log does not record receipt of the shutdown signal")
	}
	if !strings.Contains(processOutput.String(), "Application closed successfully") {
		t.Errorf("app output does not report successful shutdown: %s", processOutput.String())
	}
}

func runAppHelper(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv(appPortEnv))
	if err != nil || port < 1 {
		t.Fatalf("invalid helper port %q", os.Getenv(appPortEnv))
	}

	configuration := server.ConfigDefault
	configuration.Host = "127.0.0.1"
	configuration.Port = port
	application := gorest.New()
	application.Use(configuration)
	application.RegisterServices([]gorest.RouterRegistrar{
		func(app *gorest.App) {
			app.Get("/ready", func(ctx *gorest.Context) error {
				return ctx.SendString("ready")
			})
			app.Get("/slow", func(ctx *gorest.Context) error {
				if err := os.WriteFile(os.Getenv(appStartedFileEnv), []byte("started"), 0o600); err != nil {
					return err
				}
				time.Sleep(750 * time.Millisecond)
				return ctx.SendString("completed")
			})
		},
	})
	application.Start()
}

func waitForHTTP(t *testing.T, endpoint string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(endpoint)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("GOREST endpoint %s did not become ready", endpoint)
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("request handler did not create startup marker %s", path)
}

func testContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}
