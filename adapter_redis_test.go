package gorest

import (
	"testing"
)

func TestRedisReturnsErrorWhenAppHasNoRedisService(t *testing.T) {
	app := &App{}

	client, message, err := app.Redis()
	if err == nil {
		t.Fatal("Redis() error = nil, want error when Redis is not configured")
	}
	if client != nil {
		t.Fatalf("Redis() client = %v, want nil", client)
	}
	if message == "" {
		t.Fatal("Redis() message is empty, want troubleshooting guidance")
	}
}

func TestRedisReturnsErrorForNilApp(t *testing.T) {
	var app *App

	client, message, err := app.Redis()
	if err == nil {
		t.Fatal("Redis() error = nil, want error for nil app")
	}
	if client != nil {
		t.Fatalf("Redis() client = %v, want nil", client)
	}
	if message == "" {
		t.Fatal("Redis() message is empty, want troubleshooting guidance")
	}
}
