package redis

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/redis/go-redis/v9"
)

func (redis *Redis) Redis() (*redis.Client, string, int, error) {
	if redis == nil || redis.client == nil {
		message := "Register the Redis config with app.Use(redis.Run(...))."

		return nil, message, http.StatusNotImplemented, errors.New("redis service is not running")
	}

	return redis.client, "", http.StatusOK, nil
}

func (redis *Redis) Ping(ctx context.Context) error {
	if redis == nil || redis.client == nil {
		return errors.New("redis service is not running")
	}
	if ctx == nil {
		return errors.New("redis ping context is nil")
	}

	return redis.client.Ping(ctx).Err()
}

func (redis *Redis) Close() (string, error) {
	if redis == nil || redis.client == nil {
		return "Register the Redis config with app.Use(redis.Run(...)).", errors.New("redis service is not running")
	}

	if err := redis.client.Close(); err != nil {
		message := "failed to close redis connection gracefully"

		return message, fmt.Errorf("redis close error: %w", err)
	}

	return "Redis connection closed successfully", nil
}
