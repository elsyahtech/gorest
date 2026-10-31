//go:build integration && integrationcontainers

package redis_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	gorestredis "github.com/elsyahtech/gorest/redis"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var managedRedisContainer testcontainers.Container

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor: wait.ForLog("Ready to accept connections").
				WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "start managed Redis integration container: %v\n", err)
		os.Exit(1)
	}

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get managed Redis container host: %v\n", err)
		terminateRedisContainer(container)
		os.Exit(1)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get managed Redis container port: %v\n", err)
		terminateRedisContainer(container)
		os.Exit(1)
	}
	managedRedisContainer = container
	for name, value := range map[string]string{
		"GOREST_IT_REDIS_HOST":     host,
		"GOREST_IT_REDIS_PORT":     port.Port(),
		"GOREST_IT_REDIS_DATABASE": "0",
		"GOREST_IT_REDIS_TLS":      "false",
	} {
		if err := os.Setenv(name, value); err != nil {
			fmt.Fprintf(os.Stderr, "configure %s for managed Redis tests: %v\n", name, err)
			terminateRedisContainer(container)
			os.Exit(1)
		}
	}

	exitCode := m.Run()
	if err := container.Terminate(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "terminate managed Redis integration container: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func TestRedisReconnectsAfterServerDropsClientConnections(t *testing.T) {
	if managedRedisContainer == nil {
		t.Fatal("managed Redis integration container is unavailable")
	}

	config := gorestredis.ConfigDefault
	config.Host = os.Getenv("GOREST_IT_REDIS_HOST")
	config.Port = os.Getenv("GOREST_IT_REDIS_PORT")
	config.Timeout = time.Second
	config.MinIdleConns = 1
	config.MaxRetries = 3

	app := gorest.New()
	app.Use(gorestredis.Run(config))
	session := app.Redis()
	client, message, status, err := session.QueryRedis()
	if err != nil {
		t.Fatalf("initialize Redis client (HTTP %d, %s): %v", status, message, err)
	}
	defer func() {
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close application after Redis outage test: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping Redis before outage: %v", err)
	}

	exitCode, output, err := managedRedisContainer.Exec(ctx, []string{
		"redis-cli", "CLIENT", "KILL", "TYPE", "normal", "SKIPME", "yes",
	})
	if err != nil {
		t.Fatalf("drop existing Redis client connections: %v", err)
	}
	outputText, err := io.ReadAll(output)
	if err != nil {
		t.Fatalf("read Redis connection-reset output: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("drop existing Redis client connections exited %d: %s", exitCode, outputText)
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 10*time.Second)
	defer recoveryCancel()
	for {
		pingCtx, pingCancel := context.WithTimeout(recoveryCtx, time.Second)
		err = client.Ping(pingCtx).Err()
		pingCancel()
		if err == nil {
			break
		}
		select {
		case <-recoveryCtx.Done():
			t.Fatalf("Redis client did not reconnect after server dropped its connections: %v", err)
		case <-ticker.C:
		}
	}

	key := fmt.Sprintf("gorest:recovery:%d", time.Now().UnixNano())
	if err := client.Set(recoveryCtx, key, "reconnected", time.Minute).Err(); err != nil {
		t.Fatalf("write through reconnected Redis client: %v", err)
	}
	if got, err := client.Get(recoveryCtx, key).Result(); err != nil {
		t.Fatalf("read through reconnected Redis client: %v", err)
	} else if got != "reconnected" {
		t.Fatalf("reconnected Redis GET = %q, want %q", got, "reconnected")
	}
	if err := client.Del(recoveryCtx, key).Err(); err != nil {
		t.Errorf("delete recovery test key: %v", err)
	}
}

func TestRedisRecoversAfterTemporaryServicePause(t *testing.T) {
	if managedRedisContainer == nil {
		t.Fatal("managed Redis integration container is unavailable")
	}

	config := gorestredis.ConfigDefault
	config.Host = os.Getenv("GOREST_IT_REDIS_HOST")
	config.Port = os.Getenv("GOREST_IT_REDIS_PORT")
	config.Timeout = time.Second
	config.MinIdleConns = 1
	config.MaxRetries = 3

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	app := gorest.New()
	app.Use(gorestredis.Run(config))
	t.Cleanup(func() {
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close application after Redis service restart test: %v", err)
		}
	})

	session := app.Redis()
	client, message, status, err := session.QueryRedis()
	if err != nil {
		t.Fatalf("initialize Redis client (HTTP %d, %s): %v", status, message, err)
	}

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping Redis before service restart: %v", err)
	}

	exitCode, output, err := managedRedisContainer.Exec(ctx, []string{
		"redis-cli", "CLIENT", "PAUSE", "5000", "ALL",
	})
	if err != nil {
		t.Fatalf("pause managed Redis clients: %v", err)
	}
	outputText, err := io.ReadAll(output)
	if err != nil {
		t.Fatalf("read Redis pause command output: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("pause Redis clients exited %d: %s", exitCode, outputText)
	}

	outageCtx, outageCancel := context.WithTimeout(context.Background(), 2*time.Second)
	err = client.Ping(outageCtx).Err()
	outageCancel()
	if err == nil {
		t.Fatal("Redis client remained usable while the managed service was paused")
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := client.Ping(ctx).Err(); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Redis client did not recover after the service pause ended: %v", ctx.Err())
		case <-ticker.C:
		}
	}

	key := fmt.Sprintf("gorest:service-pause:%d", time.Now().UnixNano())
	if err := client.Set(ctx, key, "restarted", time.Minute).Err(); err != nil {
		t.Fatalf("write through recovered Redis client: %v", err)
	}
	if got, err := client.Get(ctx, key).Result(); err != nil {
		t.Fatalf("read through recovered Redis client: %v", err)
	} else if got != "restarted" {
		t.Fatalf("recovered Redis GET = %q, want %q", got, "restarted")
	}
	if err := client.Del(ctx, key).Err(); err != nil {
		t.Errorf("delete service-pause test key: %v", err)
	}
}

func terminateRedisContainer(container testcontainers.Container) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminate managed Redis integration container: %v\n", err)
	}
}
