package gorest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

func TestAppCloseGracefullyShutsDownHTTPServer(t *testing.T) {
	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve HTTP port: %v", err)
	}
	port := portListener.Addr().(*net.TCPAddr).Port
	if err := portListener.Close(); err != nil {
		t.Fatalf("release HTTP port: %v", err)
	}

	config := server.ConfigDefault
	config.Host = "127.0.0.1"
	config.Port = port
	httpServer, _, err := server.New(&config)
	if err != nil {
		t.Fatalf("initialize HTTP server: %v", err)
	}

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	httpServer.SRV.Get("/slow", func(ctx fiber.Ctx) error {
		close(handlerStarted)
		<-releaseHandler
		return ctx.SendString("completed")
	})

	app := &App{server: httpServer}
	httpServer.ExpectListen()
	listenDone := make(chan error, 1)
	go func() {
		listenDone <- httpServer.Listen(
			net.JoinHostPort(config.Host, strconv.Itoa(config.Port)),
			fiber.ListenConfig{DisableStartupMessage: true},
		)
	}()

	readyCtx, cancelReady := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelReady()
	if err := httpServer.WaitForListening(readyCtx); err != nil {
		t.Fatalf("HTTP listener did not become ready: %v", err)
	}

	endpoint := fmt.Sprintf("http://%s/slow", net.JoinHostPort(config.Host, strconv.Itoa(config.Port)))
	client := &http.Client{Timeout: 5 * time.Second}
	var responseErr error
	responseDone := make(chan struct{})
	go func() {
		defer close(responseDone)
		response, err := client.Get(endpoint)
		if err != nil {
			responseErr = err
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			responseErr = fmt.Errorf("HTTP status = %d, want %d", response.StatusCode, http.StatusOK)
		}
	}()

	select {
	case <-handlerStarted:
	case <-time.After(3 * time.Second):
		close(releaseHandler)
		t.Fatal("HTTP handler did not start")
	}

	closeDone := make(chan error, 1)
	go func() {
		_, _, err := app.Close()
		closeDone <- err
	}()

	select {
	case err := <-closeDone:
		close(releaseHandler)
		t.Fatalf("app.Close() returned before in-flight request completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseHandler)

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("app.Close() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("app.Close() did not finish after the in-flight request completed")
	}

	select {
	case err := <-listenDone:
		if err != nil {
			t.Fatalf("HTTP listener returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP listener did not return after app.Close()")
	}

	select {
	case <-responseDone:
		if responseErr != nil {
			t.Fatalf("in-flight request failed: %v", responseErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight HTTP request did not finish")
	}
}
