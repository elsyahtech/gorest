package gorest

import (
	dbimpr "github.com/elsyahtech/gorest/database"
	logimpr "github.com/elsyahtech/gorest/log"
	redisimpr "github.com/elsyahtech/gorest/redis"
	serverimpr "github.com/elsyahtech/gorest/server"
)

type Config struct {
	//nolint:revive
	log *logimpr.Config `json:"-"`

	//nolint:revive
	server *serverimpr.Config `json:"-"`

	//nolint:revive
	redis *redisimpr.Config `json:"-"`

	//nolint:revive
	database *dbimpr.Config `json:"-"`

	// This function allows to setup app name for the app
	//
	// Default: gorest
	AppName string `json:"appName"`

	// This function allows to setup app version for the app
	//
	// Default: 1.0.0
	Version string `json:"version"`

	// This function allows to setup the application environment mode.
	// Common values: "development", "staging", "production".
	// Default: development
	Environment string `json:"environment"`

	// This function allows to setup app timezone for the app
	// Timezone specifies the location for time.Time values between a string including database.
	// This setting affects how application handles and interprets time (DATETIME and TIMESTAMP), etc.
	// Common values: "UTC", "Asia/Jakarta", "Local", or any IANA Time Zone application name.
	// If not set, application will use the server's default timezone.
	// Example: "Asia/Jakarta" for Southeast Asia servers.
	// Reference: https://dev.mysql.com/doc/refman/8.0/en/time-zone-support.html
	// Default: UTC
	Timezone string `json:"timezone"`

	// This function allows to enable or disable debug mode globally.
	// Default: false
	Debug bool `json:"debug"`
}

var ConfigDefault = Config{
	log:         &logimpr.ConfigDefault,
	server:      &serverimpr.ConfigDefault,
	redis:       nil,
	database:    nil,
	AppName:     DefaultAppName,
	Version:     DefaultVersion,
	Environment: DefaultEnvironment,
	Timezone:    DefaultTimezone,
	Debug:       false,
}

func configDefault(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	cfg := config[0]

	cfg.log = ConfigDefault.log
	cfg.server = ConfigDefault.server
	cfg.redis = ConfigDefault.redis
	cfg.database = ConfigDefault.database

	if cfg.AppName == "" {
		cfg.AppName = ConfigDefault.AppName
	}

	if cfg.Version == "" {
		cfg.Version = ConfigDefault.Version
	}

	if cfg.Timezone == "" {
		cfg.Timezone = ConfigDefault.Timezone
	}

	if !cfg.Debug {
		cfg.Debug = ConfigDefault.Debug
	}

	return cfg
}
