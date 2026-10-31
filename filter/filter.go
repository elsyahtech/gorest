package filter

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func Run(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	cfg := config[0]

	return configDefault(cfg)
}

func New(config *Config) (*jwt.Token, string, error) {
	if config == nil {
		const message = "ensure the filter configuration is set up correctly and " +
			"the signature key is provided."

		return nil, message, errors.New("gorest error configuring filter")
	}

	if config.AccessTokenDuration < time.Second || config.RefreshTokenDuration < time.Second {
		const message = "ensure access and refresh token durations are at least one second"
		return nil, message, errors.New("JWT token duration must be at least one second")
	}

	method := jwt.GetSigningMethod(config.SigningMethod)
	if method == nil {
		message := "ensure the filter signing method is a supported JWT algorithm"
		return nil, message, errors.New("unsupported JWT signing method")
	}

	return jwt.New(method), "JWT initialized successfully", nil
}
