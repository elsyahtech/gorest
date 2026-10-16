package redis

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/redis/go-redis/v9"
)

func (redis *Redis) Redis() (*redis.Client, string, int, error) {
	if redis == nil || redis.client == nil {
		message := "ensure that you have run redis.Run(redis.Config{...} in your app"

		return nil, message, http.StatusNotImplemented, errors.New("redis has not been run in your app")
	}

	return redis.client, "", http.StatusOK, nil
}

func (redis *Redis) Close() (string, error) {
	if redis == nil || redis.client == nil {
		return "ensure that you have run redis.Run(redis.Config{...}) in your app", errors.New("redis has not been run in your app")
	}

	if err := redis.client.Close(); err != nil {
		message := "failed to close redis connection gracefully"

		return message, fmt.Errorf("redis close error: %w", err)
	}

	return "Redis connection closed successfully", nil
}
