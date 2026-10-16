//go:build integration

package redis_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	gorestredis "github.com/elsyahtech/gorest/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestRedisOperationsPoolTimeoutAndApplicationShutdown(t *testing.T) {
	config := integrationConfig(t)
	preflightRedis(t, config)

	app := gorest.New()
	app.Use(gorestredis.Run(config))
	closed := false
	t.Cleanup(func() {
		if !closed {
			if _, _, err := app.Close(); err != nil {
				t.Errorf("close integration application: %v", err)
			}
		}
	})

	session := app.Redis()
	ctx, cancel := session.NewContext(context.Background())
	defer cancel()

	client, message, code, err := session.QueryRedis()
	if err != nil {
		t.Fatalf("QueryRedis() failed (HTTP %d, %s): %v", code, message, err)
	}

	key := uniqueKey(t)
	t.Cleanup(func() {
		if closed {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		if err := client.Del(cleanupCtx, key).Err(); err != nil {
			t.Errorf("delete integration key: %v", err)
		}
	})

	value := "redis-integration-value"
	if err := client.Set(ctx, key, value, time.Minute).Err(); err != nil {
		t.Fatalf("SET failed: %v", err)
	}
	if got, err := client.Get(ctx, key).Result(); err != nil {
		t.Fatalf("GET failed: %v", err)
	} else if got != value {
		t.Fatalf("GET = %q, want %q", got, value)
	}

	workerKeys := make([]string, 12)
	for i := range workerKeys {
		workerKeys[i] = uniqueKey(t)
	}
	t.Cleanup(func() {
		if closed {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		if err := client.Del(cleanupCtx, workerKeys...).Err(); err != nil {
			t.Errorf("delete concurrent integration keys: %v", err)
		}
	})

	testConcurrentPoolUse(t, session, client, workerKeys)
	testConfiguredOperationTimeout(t, session, client)
	testParentContextDeadline(t, session, client)

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
	if err := client.Del(cleanupCtx, append(workerKeys, key)...).Err(); err != nil {
		t.Errorf("delete integration keys before shutdown: %v", err)
	}
	cleanupCancel()

	_, status, err := app.Close()
	closed = true
	if err != nil {
		t.Fatalf("application shutdown failed (HTTP %d): %v", status, err)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), time.Second)
	defer pingCancel()
	if err := client.Ping(pingCtx).Err(); err == nil {
		t.Fatal("Redis client remained usable after app.Close()")
	}
}

func TestRedisStartupFailsAgainstUnavailableEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local TCP port: %v", err)
	}
	address := listener.Addr().(*net.TCPAddr)
	if err := listener.Close(); err != nil {
		t.Fatalf("release local TCP port: %v", err)
	}

	config := integrationConfig(t)
	config.Host = "127.0.0.1"
	config.Port = strconv.Itoa(address.Port)
	config.TLS = false
	config.Timeout = 300 * time.Millisecond
	config.MaxRetries = 0

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client, message, err := gorestredis.New(ctx, &config)
	if err == nil {
		if client != nil {
			_, closeErr := client.Close()
			if closeErr != nil {
				t.Errorf("close unexpectedly initialized Redis client: %v", closeErr)
			}
		}
		t.Fatal("Redis.New() succeeded against a recently released local port")
	}
	if client != nil {
		t.Fatal("Redis.New() returned a client after startup ping failed")
	}
	if message == "" {
		t.Fatal("Redis.New() returned no diagnostic message after startup ping failed")
	}
}

func TestRedisTLSRejectsUntrustedServerCertificate(t *testing.T) {
	config := integrationConfig(t)
	if !config.TLS {
		t.Skip("set GOREST_IT_REDIS_TLS=true and configure a TLS-enabled Redis endpoint")
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate untrusted CA key: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "GOREST integration test untrusted CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create untrusted CA certificate: %v", err)
	}
	caPath := filepath.Join(t.TempDir(), "untrusted-ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificateDER,
	}), 0o600); err != nil {
		t.Fatalf("write untrusted CA certificate: %v", err)
	}

	config.SSLCAPath = caPath
	config.SSLCertPath = ""
	config.SSLKeyPath = ""
	config.Timeout = time.Second
	config.MaxRetries = 0
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, message, err := gorestredis.New(ctx, &config)
	if err == nil {
		if client != nil {
			if _, closeErr := client.Close(); closeErr != nil {
				t.Errorf("close client connected with an untrusted CA: %v", closeErr)
			}
		}
		t.Fatal("Redis.New() accepted a server certificate signed by an untrusted CA")
	}
	if client != nil {
		t.Fatal("Redis.New() returned a client after TLS verification failed")
	}
	if message == "" {
		t.Fatal("Redis.New() returned no diagnostic message after TLS verification failed")
	}
}

func integrationConfig(t *testing.T) gorestredis.Config {
	t.Helper()

	config := gorestredis.ConfigDefault
	config.Host = environmentValue("GOREST_IT_REDIS_HOST", config.Host)
	config.Port = environmentValue("GOREST_IT_REDIS_PORT", config.Port)
	config.Username = os.Getenv("GOREST_IT_REDIS_USERNAME")
	config.Password = os.Getenv("GOREST_IT_REDIS_PASSWORD")
	config.Enabled = true
	config.Timeout = 300 * time.Millisecond
	config.PoolSize = 4
	config.MinIdleConns = 1
	config.MaxActiveConns = 4
	config.MaxRetries = 3

	if raw := strings.TrimSpace(os.Getenv("GOREST_IT_REDIS_DATABASE")); raw != "" {
		database, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("parse GOREST_IT_REDIS_DATABASE: %v", err)
		}
		config.Database = database
	}

	if raw := strings.TrimSpace(os.Getenv("GOREST_IT_REDIS_TLS")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			t.Fatalf("parse GOREST_IT_REDIS_TLS: %v", err)
		}
		config.TLS = enabled
	}

	config.SSLCAPath = os.Getenv("GOREST_IT_REDIS_SSL_CA")
	config.SSLCertPath = os.Getenv("GOREST_IT_REDIS_SSL_CERT")
	config.SSLKeyPath = os.Getenv("GOREST_IT_REDIS_SSL_KEY")

	return gorestredis.Run(config)
}

func preflightRedis(t *testing.T, config gorestredis.Config) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, message, err := gorestredis.New(ctx, &config)
	if err != nil {
		t.Fatalf("connect to Redis integration service at %s:%s: %s: %v", config.Host, config.Port, message, err)
	}

	if _, err := client.Close(); err != nil {
		t.Fatalf("close Redis preflight client: %v", err)
	}
}

func testConcurrentPoolUse(t *testing.T, session *gorest.RedisSession, client *goredis.Client, keys []string) {
	t.Helper()

	const (
		workers = 12
		rounds  = 4
	)
	var wait sync.WaitGroup
	errs := make(chan error, workers)

	for worker := 0; worker < workers; worker++ {
		worker := worker
		key := keys[worker]
		wait.Add(1)
		go func() {
			defer wait.Done()

			for round := 0; round < rounds; round++ {
				ctx, cancel := session.NewContext(context.Background())
				value := fmt.Sprintf("%d:%d", worker, round)
				err := client.Set(ctx, key, value, time.Minute).Err()
				if err == nil {
					var got string
					got, err = client.Get(ctx, key).Result()
					if err == nil && got != value {
						err = fmt.Errorf("GET returned %q, want %q", got, value)
					}
				}
				cancel()
				if err != nil {
					errs <- fmt.Errorf("worker %d round %d: %w", worker, round, err)
					return
				}
			}
		}()
	}

	wait.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	stats := client.PoolStats()
	if stats.TotalConns > 4 {
		t.Errorf("Redis pool opened %d connections, configured maximum is 4", stats.TotalConns)
	}
}

func testConfiguredOperationTimeout(t *testing.T, session *gorest.RedisSession, client *goredis.Client) {
	t.Helper()

	ctx, cancel := session.NewContext(context.Background())
	defer cancel()

	started := time.Now()
	_, err := client.BLPop(ctx, 0, uniqueKey(t)).Result()
	if err == nil {
		t.Fatal("blocking Redis command unexpectedly succeeded without a list element")
	}
	var timeoutErr net.Error
	if !errors.Is(err, context.DeadlineExceeded) &&
		(!errors.As(err, &timeoutErr) || !timeoutErr.Timeout()) {
		t.Fatalf("blocking Redis command error = %v, want a context or I/O timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("configured Redis operation timeout took %s, want less than 2s", elapsed)
	}
}

func testParentContextDeadline(t *testing.T, session *gorest.RedisSession, client *goredis.Client) {
	t.Helper()

	parent, parentCancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer parentCancel()
	ctx, cancel := session.NewContext(parent)
	defer cancel()

	started := time.Now()
	_, err := client.BLPop(ctx, 0, uniqueKey(t)).Result()
	if err == nil {
		t.Fatal("blocking Redis command unexpectedly succeeded without a list element")
	}
	if elapsed := time.Since(started); elapsed >= 250*time.Millisecond {
		t.Fatalf("Redis command ignored the earlier parent context deadline; elapsed %s", elapsed)
	}
}

func environmentValue(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func uniqueKey(t *testing.T) string {
	t.Helper()

	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		t.Fatalf("generate unique Redis key: %v", err)
	}
	return "gorest:integration:" + hex.EncodeToString(bytes)
}
