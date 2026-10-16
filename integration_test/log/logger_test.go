//go:build integration

package log_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gorestlog "github.com/elsyahtech/gorest/log"
	"go.uber.org/zap"
)

func TestLoggerRedactsAndRotatesConfiguredFileOutput(t *testing.T) {
	config := gorestlog.Config{
		LogDir:      t.TempDir(),
		LogFile:     "integration.log",
		MaxFileSize: 1,
		MaxBackups:  2,
		MaxAge:      1,
	}
	logger, message, err := gorestlog.New(&config, "logger-integration")
	if err != nil {
		t.Fatalf("log.New() error = %v, message = %q", err, message)
	}

	password := "integration-password-secret"
	token := "headerpart.payloadpart.signaturepart"
	email := "integration@example.test"
	payload := strings.Repeat("x", 300*1024)
	for i := 0; i < 8; i++ {
		logger.Log(map[string]any{"payload": payload, "entry": i}).Info("rotation fixture")
	}
	logger.Log(map[string]any{
		"password": password,
		"email":    email,
	}).With(
		zap.String("Authorization", "Bearer "+token),
	).Error("integration log entry: password=" + password + " token " + token + " email " + email)

	if _, _, err := logger.Close(); err != nil {
		t.Fatalf("logger.Close() error = %v", err)
	}

	entries, err := os.ReadDir(config.LogDir)
	if err != nil {
		t.Fatalf("read logger output directory: %v", err)
	}
	var (
		backupCount int
		logContents strings.Builder
	)
	for _, entry := range entries {
		path := filepath.Join(config.LogDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read log output %q: %v", path, err)
		}
		logContents.Write(content)
		if entry.Name() != config.LogFile {
			backupCount++
		}
	}
	if backupCount == 0 || backupCount > config.MaxBackups {
		t.Fatalf("rotated backup count = %d, want 1..%d", backupCount, config.MaxBackups)
	}

	output := logContents.String()
	for _, sensitive := range []string{password, token, email} {
		if strings.Contains(output, sensitive) {
			t.Errorf("rotated logger output contains sensitive value %q", sensitive)
		}
	}
	for _, expected := range []string{"xxxxx", "integration log entry", "rotation fixture"} {
		if !strings.Contains(output, expected) {
			t.Errorf("combined rotated logs do not contain %q", expected)
		}
	}
}
