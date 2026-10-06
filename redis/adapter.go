package redis

import (
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func (redis *Redis) Redis() (*redis.Client, string, error) {
	if redis == nil || redis.client == nil {
		message := "ensure that you have run redis.Run(redis.Config{...} in your app"

		return nil, message, errors.New("redis has not been run in your app")
	}

	return redis.client, "", nil
}

func (redis *Redis) Close() (string, error) {
	if redis == nil || redis.client == nil {
		return "ensure that you have run redis.Run(redis.Config{...}) in your app", errors.New("redis has not been run in your app")
	}

	if err := redis.client.Close(); err != nil {
		message := "failed to close redis connection gracefully"

		return message, fmt.Errorf("redis close error: %w", err)
	}

	return "", nil
}
