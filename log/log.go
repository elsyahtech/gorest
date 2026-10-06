//nolint:revive
package log

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Run(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	cfg := config[0]

	return configDefault(cfg)
}

func New(config *Config, appName string) (*Log, string, error) {
	if config == nil {
		//nolint:revive
		config = &ConfigDefault
	}

	var log Log

	log.logger = zap.NewNop()

	logDir := config.LogDir

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		message := fmt.Sprintf("Log Initialize: ensure %s directory can be created and is writable", logDir)

		return nil, message, fmt.Errorf("create log directory: %w", err)
	}

	logFile := config.LogFile
	filePath := filepath.Join(logDir, logFile)

	file, closeFn, err := zap.Open(filePath)
	if err != nil {
		const message = "Log Initialize: ensure log file can be created and writable"

		return nil, message, fmt.Errorf("open log file: %w", err)
	}

	fileEncoder := zapcore.NewJSONEncoder(newEncoderConfig())
	core := zapcore.NewCore(fileEncoder, file, zapcore.InfoLevel)
	fields := map[string]any{"service": appName}
	zapFields := make([]zap.Field, 0, len(fields))

	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	if config.Caller {
		log.logger = zap.New(core, zap.AddCaller(), zap.Fields(zapFields...))
	} else {
		log.logger = zap.New(core, zap.Fields(zapFields...))
	}

	_ = closeFn

	return &log, "", nil
}
