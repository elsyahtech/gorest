//nolint:revive,nolintlint
package log

import corelog "log"

type Config struct {
	// LogDir defines the root directory path where log files will be stored.
	// If the directory does not exist, it should be created automatically on startup.
	// Example: "./logs" or "/var/log/myapp"
	// Default: "./logs"
	LogDir string

	// InfoFile is the filename for general/info level logs.
	// Example: "app.log"
	// Default: "app.log"
	LogFile string

	// Caller enables or disables the inclusion of caller information (such as
	// the file name and line number) in the log entries.
	// Example: true
	// Default: true
	Caller bool

	// MaxFileSize defines the maximum size in megabytes (MB) that a log file can reach
	// before it gets rotated (split into a new file).
	// Example: 100 (means 100 MB)
	// Default: 100
	MaxFileSize int

	// MaxBackups defines the maximum number of old log files to retain after rotation.
	// 0 means retain all old log files (not recommended for disk space management).
	// Example: 5 (keep up to 5 old backup files)
	// Default: 5
	MaxBackups int

	// MaxAge defines the maximum number of days to retain old log files based on
	// the timestamp encoded in their filename.
	// 0 means forever (or controlled solely by MaxBackups).
	// Example: 7 (delete logs older than 7 days)
	// Default: 7
	MaxAge int
}

var ConfigDefault = Config{
	// Default: "./logs"
	LogDir: DefaultLogDir,

	// Default: "info.log"
	LogFile: DefaultLogFile,

	Caller: DefaultCaller,

	// Default: 100 (MB)
	MaxFileSize: DefaultMaxFileSize,

	// Default: 5 (keep up to 5 old backup files)
	MaxBackups: DefaultMaxBackups,

	// Default: 7 (delete logs older than 7 days)
	MaxAge: DefaultLogMaxAge,
}

func configDefault(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	// Override default config
	cfg := config[0]

	// if LogDir not configured, set default to "./logs"
	if cfg.LogDir == "" {
		cfg.LogDir = ConfigDefault.LogDir
	}

	// if LogFile not configured, set default to "app.log"
	if cfg.LogFile == "" {
		cfg.LogFile = ConfigDefault.LogFile
	}

	// if MaxFileSize not configured, set default to 100 (MB)
	if cfg.MaxFileSize == 0 {
		cfg.MaxFileSize = ConfigDefault.MaxFileSize
	}

	if cfg.MaxFileSize < 0 {
		corelog.Fatalf("Config Log: MaxFileSize must be greater than or equal to 0") //nolint:revive
	}

	// if MaxBackups not configured, set default to 5 (keep up to 5 old backup files)
	if cfg.MaxBackups == 0 {
		cfg.MaxBackups = ConfigDefault.MaxBackups
	}

	if cfg.MaxBackups < 0 {
		corelog.Fatalf("Config Log: MaxBackups must be greater than or equal to 0") //nolint:revive
	}

	// if MaxAge not configured, set default to 7 (delete logs older than 7 days)
	if cfg.MaxAge == 0 {
		cfg.MaxAge = ConfigDefault.MaxAge
	}

	if cfg.MaxAge < 0 {
		corelog.Fatalf("Config Log: MaxAge must be greater than or equal to 0") //nolint:revive
	}

	return cfg
}
