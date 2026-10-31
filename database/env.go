package database

import (
	"os"
	"strings"
)

// ConfigFromEnv creates a database configuration from environment variables.
// Passwords and private keys must be supplied by the process environment or a
// secret manager; the framework never logs secret values.
func ConfigFromEnv(prefix string) Config {
	prefix = strings.TrimRight(prefix, "_") + "_"
	config := ConfigDefault
	config.Driver = envOr(prefix+"DRIVER", config.Driver)
	config.Host = envOr(prefix+"HOST", config.Host)
	config.Port = envOr(prefix+"PORT", config.Port)
	config.Name = envOr(prefix+"NAME", config.Name)
	config.Username = envOr(prefix+"USERNAME", config.Username)
	config.Password = os.Getenv(prefix + "PASSWORD")
	config.AuthSource = envOr(prefix+"AUTH_SOURCE", config.AuthSource)
	config.TLS = envOr(prefix+"TLS", config.TLS)
	config.SSLCAPath = envOr(prefix+"SSL_CA", config.SSLCAPath)
	config.SSLCertPath = envOr(prefix+"SSL_CERT", config.SSLCertPath)
	config.SSLKeyPath = envOr(prefix+"SSL_KEY", config.SSLKeyPath)
	return Run(config)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
