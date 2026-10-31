package redis

import (
	"context"
	"net/http"
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

func TestRedisAdapterHandlesUnavailableAndInitializedClients(t *testing.T) {
	var missing *Redis
	if client, message, code, err := missing.Redis(); client != nil || message == "" || code != http.StatusNotImplemented || err == nil {
		t.Fatalf("nil Redis.Redis() = (%v, %q, %d, %v)", client, message, code, err)
	}
	if message, err := missing.Close(); message == "" || err == nil {
		t.Fatalf("nil Redis.Close() = (%q, %v)", message, err)
	}
	if err := missing.Ping(context.Background()); err == nil {
		t.Fatal("nil Redis.Ping() error = nil, want unavailable-service error")
	}

	client := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:6379"})
	adapter := &Redis{client: client}
	got, message, code, err := adapter.Redis()
	if got != client || message != "" || code != http.StatusOK || err != nil {
		t.Fatalf("initialized Redis.Redis() = (%v, %q, %d, %v)", got, message, code, err)
	}
	if err := adapter.Ping(nil); err == nil {
		t.Fatal("initialized Redis.Ping(nil) error = nil, want nil-context error")
	}
	if message, err := adapter.Close(); message != "Redis connection closed successfully" || err != nil {
		t.Fatalf("initialized Redis.Close() = (%q, %v)", message, err)
	}
}
