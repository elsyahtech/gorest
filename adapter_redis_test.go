package gorest

import (
	"context"
	"testing"
	"time"

	gorestredis "github.com/elsyahtech/gorest/redis"
)

func TestRedisReturnsErrorWhenAppHasNoRedisService(t *testing.T) {
	app := &RedisSession{}

	client, message, httpCode, err := app.QueryRedis()
	if err == nil {
		t.Fatal("QueryRedis() error = nil, want error when Redis is not configured")
	}
	if client != nil {
		t.Fatalf("QueryRedis() client = %v, want nil", client)
	}
	if message == "" {
		t.Fatal("QueryRedis() message is empty, want troubleshooting guidance")
	}
	if httpCode == 0 {
		t.Fatal("QueryRedis() httpCode is 0, want HTTP status code")
	}
}

func TestRedisReturnsErrorForNilApp(t *testing.T) {
	var app *RedisSession

	client, message, httpCode, err := app.QueryRedis()
	if err == nil {
		t.Fatal("QueryRedis() error = nil, want error for nil app")
	}
	if client != nil {
		t.Fatalf("QueryRedis() client = %v, want nil", client)
	}
	if message == "" {
		t.Fatal("QueryRedis() message is empty, want troubleshooting guidance")
	}
	if httpCode == 0 {
		t.Fatal("QueryRedis() httpCode is 0, want HTTP status code")
	}
}

func TestAppRedisReturnsSessionWhenAppIsNil(t *testing.T) {
	var app *App
	session := app.Redis()
	if session == nil {
		t.Fatal("Redis() = nil, want an empty session with an explicit QueryRedis error")
	}

	client, message, httpCode, err := session.QueryRedis()
	if err == nil || client != nil || message == "" || httpCode == 0 {
		t.Fatalf("QueryRedis() = (%v, %q, %d, %v), want an explicit unavailable-service error", client, message, httpCode, err)
	}
}

func TestRedisSessionNewContextAppliesConfiguredTimeout(t *testing.T) {
	session := &RedisSession{config: &gorestredis.Config{Timeout: 200 * time.Millisecond}}
	ctx, cancel := session.NewContext(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("NewContext() did not set a deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > 200*time.Millisecond {
		t.Fatalf("NewContext() deadline has %s remaining, want a positive duration no greater than 200ms", remaining)
	}
}

func TestRedisSessionNewContextPreservesEarlierParentDeadline(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer parentCancel()

	session := &RedisSession{config: &gorestredis.Config{Timeout: time.Second}}
	ctx, cancel := session.NewContext(parent)
	defer cancel()

	parentDeadline, _ := parent.Deadline()
	gotDeadline, ok := ctx.Deadline()
	if !ok || !gotDeadline.Equal(parentDeadline) {
		t.Fatalf("NewContext() deadline = %v, want parent deadline %v", gotDeadline, parentDeadline)
	}
}

func TestRedisSessionNewContextUsesBackgroundForNilContext(t *testing.T) {
	session := &RedisSession{config: &gorestredis.Config{Timeout: time.Second}}
	ctx, cancel := session.NewContext(nil)
	defer cancel()

	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("NewContext(nil) did not apply the configured timeout")
	}
}

func TestRedisSessionNewContextCanBeCancelled(t *testing.T) {
	session := &RedisSession{config: &gorestredis.Config{Timeout: time.Second}}
	ctx, cancel := session.NewContext(context.Background())
	cancel()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("NewContext() context was not cancelled by its cancel function")
	}
}
