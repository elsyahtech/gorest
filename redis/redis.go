package redis

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
)

func Run(cfg Config) Config {
	cfgValidated := configDefault(cfg)

	return cfgValidated
}

func New(ctx context.Context, config *Config) (*Redis, string, error) {
	if config == nil {
		return nil, "", nil
	}

	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%s", config.Host, config.Port),
		Username: config.Username,
		Password: config.Password,
		DB:       config.Database,

		// Timeouts
		DialTimeout:  config.Timeout,
		ReadTimeout:  config.Timeout,
		WriteTimeout: config.Timeout,

		// Connection pooling
		MaxRetries:      config.MaxRetries,
		PoolSize:        config.PoolSize,
		MinIdleConns:    config.MinIdleConns,
		ConnMaxLifetime: config.ConnMaxLifetime,
		ConnMaxIdleTime: config.ConnMaxIdleTime,
		MaxActiveConns:  config.MaxActiveConns,
		PoolTimeout:     config.PoolTimeout,
		ReadBufferSize:  config.ReadBufferSize,
		WriteBufferSize: config.WriteBufferSize,
	}

	// TLS setup (if enabled)
	if config.TLS {
		tlsConfig, msg, err := setupTLS(config)
		if err != nil {
			return nil, msg, err
		}

		opts.TLSConfig = tlsConfig
	}

	client := redis.NewClient(opts)

	if err := client.Ping(ctx).Err(); err != nil {
		const message = "ensure redis service/daemon is running and ensure network connectivity to the server is stable"

		return nil, message, fmt.Errorf("try ping to redis host: %w", err)
	}

	return &Redis{client: client}, "", nil
}

func setupTLS(config *Config) (*tls.Config, string, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
	}

	if config.SSLCAPath != "" {
		if msg, err := configSSLCAPath(config, tlsConfig); err != nil {
			return nil, msg, err
		}
	}

	if config.SSLCertPath != "" && config.SSLKeyPath != "" {
		if msg, err := configSSLCertPath(config, tlsConfig); err != nil {
			return nil, msg, err
		}
	}

	return tlsConfig, "", nil
}

func configSSLCAPath(config *Config, tlsConfig *tls.Config) (string, error) {
	caCert, err := os.ReadFile(config.SSLCAPath)
	if err != nil {
		const message = "please ensure the file path is correct, the application has read permissions, and the file is not corrupted."

		return message, fmt.Errorf("read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()

	if !caCertPool.AppendCertsFromPEM(caCert) {
		const message = "please check the contents of the certificate file."

		return message, errors.New("parse CA certificate")
	}

	tlsConfig.RootCAs = caCertPool

	return "", nil
}

func configSSLCertPath(config *Config, tlsConfig *tls.Config) (string, error) {
	cert, err := tls.LoadX509KeyPair(config.SSLCertPath, config.SSLKeyPath)
	if err != nil {
		const message = "ensure the cert/key pair is valid, paths are correct, and formats match."

		return message, fmt.Errorf("load client certificate: %w", err)
	}

	tlsConfig.Certificates = []tls.Certificate{cert}

	return "", nil
}
