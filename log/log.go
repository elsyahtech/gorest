//nolint:revive
package log

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
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

	if err := os.MkdirAll(logDir, 0o700); err != nil {
		message := fmt.Sprintf("Log Initialize: ensure %s directory can be created and is writable", logDir)

		return nil, message, fmt.Errorf("create log directory: %w", err)
	}

	writer := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, config.LogFile),
		MaxSize:    config.MaxFileSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAge,
	}
	if _, err := writer.Write(nil); err != nil {
		const message = "Log Initialize: ensure log file can be created and writable"
		_ = writer.Close()

		return nil, message, fmt.Errorf("open log file: %w", err)
	}

	fileEncoder := zapcore.NewJSONEncoder(newEncoderConfig())
	core := redactingCore{Core: zapcore.NewCore(fileEncoder, zapcore.AddSync(writer), zapcore.InfoLevel)}
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

	log.closeFn = writer.Close
	return &log, "", nil
}
