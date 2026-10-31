//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type fallbackStringer string

func (value fallbackStringer) String() string {
	return string(value)
}

func TestRunAppliesLogConfigurationDefaults(t *testing.T) {
	config := Run(Config{LogDir: t.TempDir(), Caller: false})
	if config.LogDir == "" || config.LogFile != ConfigDefault.LogFile ||
		config.MaxFileSize != ConfigDefault.MaxFileSize ||
		config.MaxBackups != ConfigDefault.MaxBackups ||
		config.MaxAge != ConfigDefault.MaxAge || config.Caller {
		t.Fatalf("Run() configuration = %#v", config)
	}

	if got := Run(); got != ConfigDefault {
		t.Fatalf("Run() without configuration = %#v, want %#v", got, ConfigDefault)
	}
}

func TestRedactFallbackHandlesOpaqueAndStringValues(t *testing.T) {
	type hiddenValue struct {
		secret string
	}

	opaque := reflect.ValueOf(hiddenValue{secret: "private"}).Field(0)
	if got := redactFallback(opaque); got != redactedValue {
		t.Fatalf("redactFallback(unexported) = %#v, want redacted marker", got)
	}

	for _, value := range []any{
		fallbackStringer("password=secret"),
		errors.New("token=secret"),
		"visible",
	} {
		got := redactFallback(reflect.ValueOf(value))
		if strings.Contains(fmt.Sprint(got), "secret") {
			t.Fatalf("redactFallback(%T) exposed a secret: %v", value, got)
		}
		if value == "visible" && got != "visible" {
			t.Fatalf("redactFallback(safe string) = %#v, want visible", got)
		}
	}
}

func TestNewRotatesLogAndAppliesBackupLimit(t *testing.T) {
	config := Config{
		LogDir:      filepath.Join(t.TempDir(), "logs"),
		LogFile:     "service.log",
		MaxFileSize: 1,
		MaxBackups:  2,
		MaxAge:      7,
	}

	logger, message, err := New(&config, "rotation-test")
	if err != nil {
		t.Fatalf("New() error = %v, message = %q", err, message)
	}

	payload := strings.Repeat("x", 300*1024)
	for i := 0; i < 8; i++ {
		logger.Log(map[string]any{"index": i, "payload": payload}).Info("rotation fixture")
	}

	if _, _, err := logger.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	entries, err := os.ReadDir(config.LogDir)
	if err != nil {
		t.Fatalf("read log directory: %v", err)
	}
	var backups int
	for _, entry := range entries {
		if entry.Name() != config.LogFile {
			backups++
		}
	}
	if backups == 0 {
		t.Fatal("expected log rotation to create at least one backup")
	}
	if backups > config.MaxBackups {
		t.Fatalf("found %d backup files, configured maximum is %d", backups, config.MaxBackups)
	}
	logPath := filepath.Join(config.LogDir, config.LogFile)
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("active log file is missing after rotation: %v", err)
	}
	logInfo, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat active log file: %v", err)
	}
	if logInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("new log file permissions = %04o, want no group/other access", logInfo.Mode().Perm())
	}
	dirInfo, err := os.Stat(config.LogDir)
	if err != nil {
		t.Fatalf("stat log directory: %v", err)
	}
	if dirInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("new log directory permissions = %04o, want no group/other access", dirInfo.Mode().Perm())
	}
}

func TestCloseCallsOutputCloserEvenWhenSyncFails(t *testing.T) {
	syncErr := errors.New("sync failed")
	closeErr := errors.New("close failed")
	writer := &failingSyncWriter{syncErr: syncErr}
	closed := false
	logger := &Log{
		logger: zap.New(zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			writer,
			zapcore.InfoLevel,
		)),
		closeFn: func() error {
			closed = true
			return closeErr
		},
	}

	_, code, err := logger.Close()
	if err == nil {
		t.Fatal("Close() error = nil, want sync and close failures")
	}
	if code != 500 {
		t.Fatalf("Close() HTTP code = %d, want 500", code)
	}
	if !closed {
		t.Fatal("Close() did not invoke the output closer after sync failed")
	}
	if !errors.Is(err, syncErr) || !errors.Is(err, closeErr) {
		t.Fatalf("Close() error = %v, want both sync and close errors", err)
	}
}

func TestNewReportsLogFileOpenFailure(t *testing.T) {
	config := Config{
		LogDir:      t.TempDir(),
		LogFile:     "invalid\x00.log",
		MaxFileSize: 1,
		MaxBackups:  1,
		MaxAge:      1,
	}

	logger, message, err := New(&config, "open-failure-test")
	if err == nil {
		if logger != nil {
			_, _, closeErr := logger.Close()
			if closeErr != nil {
				t.Errorf("close unexpectedly initialized logger: %v", closeErr)
			}
		}
		t.Fatal("New() error = nil, want log file open error")
	}
	if logger != nil {
		t.Fatal("New() returned a logger after the output could not be opened")
	}
	if message == "" {
		t.Fatal("New() message is empty, want log file troubleshooting guidance")
	}
}

func TestLogWriteFailureIsReportedAndDoesNotInterruptCaller(t *testing.T) {
	writeErr := errors.New("simulated disk write failure")
	var errorOutput bytes.Buffer
	logger := &Log{
		logger: zap.New(
			zapcore.NewCore(
				zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
				failingWriteSyncer{err: writeErr},
				zapcore.InfoLevel,
			),
			zap.ErrorOutput(zapcore.AddSync(&errorOutput)),
		),
	}

	businessContinued := false
	logger.Log(nil).Error("business operation error")
	businessContinued = true

	if !businessContinued {
		t.Fatal("logging failure interrupted the caller")
	}
	if !strings.Contains(errorOutput.String(), writeErr.Error()) {
		t.Fatalf("Zap error output = %q, want write failure details", errorOutput.String())
	}
}

func TestLoggerRedactsSensitiveFieldsNestedValuesAndMessages(t *testing.T) {
	type nestedCredentials struct {
		Password string `json:"password"`
		Email    string `json:"email"`
		Alias    string `json:"alias"`
	}
	config := Config{
		LogDir:      t.TempDir(),
		LogFile:     "redaction.log",
		MaxFileSize: 1,
		MaxBackups:  1,
		MaxAge:      1,
	}
	logger, message, err := New(&config, "redaction-test")
	if err != nil {
		t.Fatalf("New() error = %v, message = %q", err, message)
	}

	secret := "synthetic-private-value"
	jwt := "headerpart.payloadpart.signaturepart"
	email := "person@example.test"
	phone := "+1 415 555 0123"
	cookie := "sid=synthetic-session; theme=dark"
	passwordMarker := "password" + "="

	logger.Log(map[string]any{
		"password": secret,
		"nested": nestedCredentials{
			Password: secret,
			Email:    email,
			Alias:    "visible-alias",
		},
		"metadata": map[string]any{
			"refresh_token": jwt,
			"note":          "contact " + email,
		},
		"phone":  phone,
		"cookie": cookie,
	}).Error("login failed")
	logger.Log(nil).With(
		zap.String("Authorization", "Bearer "+jwt),
		zap.Error(errors.New("request failed: "+passwordMarker+secret)),
	).Error("request credentials: " + passwordMarker + secret + " token " + jwt + " email " + email + " phone " + phone + " Cookie: " + cookie)

	if _, _, err := logger.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	output, err := os.ReadFile(filepath.Join(config.LogDir, config.LogFile))
	if err != nil {
		t.Fatalf("read redaction log: %v", err)
	}
	text := string(output)
	for _, sensitive := range []string{secret, jwt, email, phone, cookie, "sid=synthetic-session"} {
		if strings.Contains(text, sensitive) {
			t.Errorf("log output contains sensitive value %q: %s", sensitive, text)
		}
	}
	for _, expected := range []string{redactedValue, "visible-alias", "login failed"} {
		if !strings.Contains(text, expected) {
			t.Errorf("log output does not contain %q: %s", expected, text)
		}
	}
}

func TestIsSensitiveKeyOnlyMatchesSensitiveNames(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{key: "payload", want: false},
		{key: "alias", want: false},
		{key: "Authorization", want: true},
		{key: "refresh-token", want: true},
		{key: "email", want: true},
		{key: "profile_email", want: true},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			if got := isSensitiveKey(test.key); got != test.want {
				t.Errorf("isSensitiveKey(%q) = %t, want %t", test.key, got, test.want)
			}
		})
	}
}

type failingSyncWriter struct {
	syncErr error
}

func (writer *failingSyncWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (writer *failingSyncWriter) Sync() error {
	return writer.syncErr
}

type failingWriteSyncer struct {
	err error
}

func (writer failingWriteSyncer) Write([]byte) (int, error) {
	return 0, writer.err
}

func (writer failingWriteSyncer) Sync() error {
	return nil
}
