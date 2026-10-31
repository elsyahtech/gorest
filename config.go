package gorest

import (
	dbimpr "github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/filter"
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

	//nolint:revive
	filter *filter.Config `json:"-"`

	// This function allows to setup app name for the app
	//
	// Default: gorest
	AppName string `json:"appName"`

	// This function allows to setup app version for the app
	//
	// Default: 1.0.0
	Version string `json:"version"`

	// This function allows to setup the application environment mode.
	// Common values: "development", "testing", "production". Custom values such
	// as "staging" are allowed; custom values use production TLS warnings.
	// Default: development
	Environment string `json:"environment"`

	// Timezone specifies the IANA location used by database drivers that support
	// an application timezone (currently MySQL and Oracle). It does not modify
	// the process-wide time.Local value.
	// Common values: "UTC", "Asia/Jakarta", "Local", or any IANA Time Zone application name.
	// If not set, the default timezone is UTC.
	// Example: "Asia/Jakarta" for Southeast Asia servers.
	// Reference: https://dev.mysql.com/doc/refman/8.0/en/time-zone-support.html
	// Default: UTC
	Timezone string `json:"timezone"`

	// This function allows to enable or disable debug mode globally.
	// Default: false
	Debug bool `json:"debug"`

	// This function allows to enable or disable insecure production mode.
	// When set to true, the application will allow insecure connections in production mode.
	// Default: false
	AllowInsecureProduction bool `json:"allow_insecure_production"`
}

var ConfigDefault = Config{
	log:                     &logimpr.ConfigDefault,
	server:                  &serverimpr.ConfigDefault,
	filter:                  &filter.ConfigDefault,
	redis:                   nil,
	database:                nil,
	AppName:                 DefaultAppName,
	Version:                 DefaultVersion,
	Environment:             Development,
	Timezone:                DefaultTimezone,
	Debug:                   false,
	AllowInsecureProduction: false,
}

func configDefault(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	cfg := config[0]

	cfg.log = ConfigDefault.log
	cfg.redis = ConfigDefault.redis
	cfg.database = ConfigDefault.database

	if cfg.AppName == "" {
		cfg.AppName = ConfigDefault.AppName
	}

	if cfg.Version == "" {
		cfg.Version = ConfigDefault.Version
	}

	if cfg.Environment == "" {
		cfg.Environment = ConfigDefault.Environment
	}

	if cfg.Timezone == "" {
		cfg.Timezone = ConfigDefault.Timezone
	}

	if !cfg.Debug {
		cfg.Debug = ConfigDefault.Debug
	}

	return cfg
}
