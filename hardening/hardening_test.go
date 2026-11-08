package hardening

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/filter"
	"github.com/elsyahtech/gorest/server"
)

func TestNewValidatesAllowlistAndDefaultsToDisabled(t *testing.T) {
	hardening, err := New(Config{})
	if err != nil {
		t.Fatalf("New(Config{}): %v", err)
	}
	if hardening.Enabled() {
		t.Fatal("hardening is enabled by default")
	}
	if hardening.AllowsPeer(nil) {
		t.Fatal("disabled hardening allowed a peer")
	}

	if _, err := New(Config{HardeningAllowedIPs: []string{"not-an-ip"}}); err == nil {
		t.Fatal("New with invalid allowlist succeeded")
	}

	enabled, err := New(Config{Enable: true, HardeningAllowedIPs: []string{"192.0.2.0/24"}})
	if err != nil {
		t.Fatalf("New with valid CIDR: %v", err)
	}
	if !enabled.AllowsPeer([]byte{192, 0, 2, 10}) {
		t.Fatal("peer in configured CIDR was denied")
	}
	if enabled.AllowsPeer([]byte{192, 0, 3, 10}) {
		t.Fatal("peer outside configured CIDR was allowed")
	}
}

func TestRunCopiesAllowlist(t *testing.T) {
	original := []string{"192.0.2.1"}
	config := Run(Config{HardeningAllowedIPs: original})
	original[0] = "198.51.100.1"
	if config.HardeningAllowedIPs[0] != "192.0.2.1" {
		t.Fatalf("allowlist changed through caller alias: %v", config.HardeningAllowedIPs)
	}
}

func TestScanInventoriesConfigAndRedactsSecrets(t *testing.T) {
	key := "do-not-export-this-signing-key"
	report := Scan("production", map[string]any{
		"app": struct {
			Debug                   bool
			AllowInsecureProduction bool
		}{Debug: true, AllowInsecureProduction: true},
		"filter.Config": filter.Config{
			SignatureKey:        &key,
			AccessTokenDuration: 15 * time.Minute,
			CookieConfig:        filter.CookieConfig{CookieSecure: false},
		},
		"server.Config": server.Config{
			Host: "0.0.0.0",
			Core: nil,
		},
	})

	if report.EvidenceMode != "effective_runtime_config" {
		t.Fatalf("evidence mode = %q", report.EvidenceMode)
	}
	if report.Summary.Fail == 0 || report.Summary.Review == 0 {
		t.Fatalf("expected findings, got summary %#v", report.Summary)
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if strings.Contains(string(encoded), key) {
		t.Fatal("report contains the configured signing key")
	}
	if !strings.Contains(string(encoded), `"field_path":"filter.Config.SignatureKey"`) {
		t.Fatal("report does not inventory the signing key field")
	}
	if !strings.Contains(string(encoded), `"field_path":"filter.Config.AccessTokenDuration","value":"15m0s"`) {
		t.Fatal("report did not include the access-token duration")
	}
	if !strings.Contains(string(encoded), `"field_path":"filter.Config.CookieConfig.DisableStoreTokenCookie","value":false`) {
		t.Fatal("report did not include the cookie storage setting")
	}
}

func TestScanDetectsIncompleteListenerTLSPair(t *testing.T) {
	report := Scan("production", map[string]any{
		"server.Config": server.Config{
			Listener: server.ListenerConfig{TLSCertKeyFile: "/secret/server.key"},
		},
	})

	for _, finding := range report.Findings {
		if finding.ID == "HTTP-002" && finding.Status == "fail" {
			encoded, err := json.Marshal(report)
			if err != nil {
				t.Fatalf("marshal report: %v", err)
			}
			if strings.Contains(string(encoded), "/secret/server.key") {
				t.Fatal("report contains private-key path")
			}
			return
		}
	}
	t.Fatal("incomplete listener TLS pair was not reported")
}

func TestDatabaseTLSAssessmentUsesDriverSpecificModes(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		mode   string
		values map[string]any
		want   string
	}{
		{name: "mysql disabled", driver: database.MYSQL, mode: "disable", want: "fail"},
		{name: "mysql preferred", driver: database.MYSQL, mode: "preferred", want: "review"},
		{name: "mysql verified", driver: database.MYSQL, mode: "true", want: "pass"},
		{name: "postgres fallback", driver: database.POSTGRES, mode: "prefer", want: "fail"},
		{name: "postgres require", driver: database.POSTGRES, mode: "require", want: "review"},
		{name: "postgres verify full", driver: database.POSTGRES, mode: "verify-full", want: "pass"},
		{name: "sqlserver trust certificate", driver: database.SQLSERVER, mode: "mandatory",
			values: map[string]any{"database.Config.TrustServerCertificate": true}, want: "fail"},
		{name: "oracle no wallet", driver: database.ORACLE, mode: "enable", want: "fail"},
		{name: "oracle with wallet", driver: database.ORACLE, mode: "enable",
			values: map[string]any{"database.Config.SSLCAPath": "/wallet"}, want: "pass"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, _, _, _ := databaseTLSAssessment(test.driver, test.mode, test.values)
			if status != test.want {
				t.Fatalf("databaseTLSAssessment status = %q, want %q", status, test.want)
			}
		})
	}
}

func TestScanHonorsGlobalInsecureProductionApproval(t *testing.T) {
	report := Scan("production", map[string]any{
		"app": struct{ AllowInsecureProduction bool }{AllowInsecureProduction: true},
		"database.Config": database.Config{
			Driver: database.POSTGRES,
			TLS:    database.TLSDisabled,
		},
	})

	for _, finding := range report.Findings {
		if finding.ID == "DB-001" {
			if finding.Status != "review" {
				t.Fatalf("DB-001 status = %q, want review when global exception is approved", finding.Status)
			}
			return
		}
	}
	t.Fatal("database TLS finding was not generated")
}

func TestRenderHTMLEscapesInventoryValues(t *testing.T) {
	report := Scan("test", map[string]any{
		"example.Config": struct{ Name string }{Name: `<script>alert("x")</script>`},
	})
	output, err := RenderHTML(report)
	if err != nil {
		t.Fatalf("render report: %v", err)
	}
	if strings.Contains(string(output), `<script>alert("x")</script>`) {
		t.Fatal("HTML report did not escape configuration values")
	}
	if !strings.Contains(string(output), "&lt;script&gt;") {
		t.Fatal("HTML report does not include escaped configuration value")
	}
}

func TestSafeReflectValueHandlesSupportedAndOpaqueValues(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		wantValue  any
		wantStatus string
	}{
		{name: "invalid", value: nil, wantStatus: "unknown"},
		{name: "boolean", value: true, wantValue: true, wantStatus: "observed"},
		{name: "string", value: "value", wantValue: "value", wantStatus: "observed"},
		{name: "integer", value: int64(4), wantValue: int64(4), wantStatus: "observed"},
		{name: "duration", value: 15 * time.Second, wantValue: "15s", wantStatus: "observed"},
		{name: "unsigned", value: uint32(9), wantValue: uint64(9), wantStatus: "observed"},
		{name: "float", value: 1.5, wantValue: 1.5, wantStatus: "observed"},
		{name: "timestamp", value: time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("test", 3600)),
			wantValue: "2026-01-02T02:04:05Z", wantStatus: "observed"},
		{name: "nested struct", value: struct{ Value int }{Value: 1}, wantValue: "not assessable", wantStatus: "unknown"},
		{name: "nil function", value: (func())(nil), wantValue: "not configured", wantStatus: "unknown"},
		{name: "function", value: func() {}, wantValue: "configured opaque value", wantStatus: "unknown"},
		{name: "nil slice", value: []string(nil), wantValue: []any(nil), wantStatus: "observed"},
		{name: "slice", value: []any{"value", 3}, wantValue: []any{"value", int64(3)}, wantStatus: "observed"},
		{name: "array", value: [2]string{"a", "b"}, wantValue: []any{"a", "b"}, wantStatus: "observed"},
		{name: "map", value: map[string]any{"name": "value", "apiKey": "secret"},
			wantValue: map[string]any{"name": "value", "apiKey": true}, wantStatus: "observed"},
		{name: "non-string map keys", value: map[int]string{1: "ignored"}, wantValue: map[string]any{}, wantStatus: "observed"},
		{name: "complex", value: complex(1, 2), wantValue: "unsupported value (complex128)", wantStatus: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotValue, gotStatus := safeReflectValue(reflect.ValueOf(test.value), "")
			if !reflect.DeepEqual(gotValue, test.wantValue) || gotStatus != test.wantStatus {
				t.Fatalf("safeReflectValue() = (%#v, %q), want (%#v, %q)",
					gotValue, gotStatus, test.wantValue, test.wantStatus)
			}
		})
	}
}

func TestAssessProductionConfigurationBranches(t *testing.T) {
	tests := []struct {
		name          string
		values        map[string]any
		wantFindings  map[string]string
		wantNoFinding []string
	}{
		{
			name: "insecure settings are identified",
			values: map[string]any{
				"app.Debug":                                          true,
				"app.AllowInsecureProduction":                        false,
				"server.Config.Host":                                 "0.0.0.0",
				"server.Config.Listener.TLSCertFile":                 map[string]bool{"configured": true},
				"server.Config.Listener.TLSCertKeyFile":              map[string]bool{"configured": false},
				"server.Config.Listener.EnablePrintRoutes":           true,
				"server.Config.Core.ReadTimeout":                     "0s",
				"server.Config.Core.WriteTimeout":                    "invalid",
				"server.Config.Core.BodyLimit":                       int64(0),
				"database.Config.Disable":                            false,
				"database.Config.Driver":                             database.MYSQL,
				"database.Config.TLS":                                database.TLSDisabled,
				"database.Config.Migration":                          true,
				"database.Config.Seeder":                             true,
				"database.Config.QueryLogging":                       true,
				"redis.Config.Disable":                               false,
				"redis.Config.TLS":                                   false,
				"filter.Config.CookieConfig.DisableStoreTokenCookie": false,
				"filter.Config.CookieConfig.CookieSecure":            false,
				"filter.Config.CookieConfig.CookieHTTPOnly":          false,
				"monitoring.Config.DisableMonitoringEndpoints":       false,
				"monitoring.Config.MonitoringAllowedIPs":             []any{},
			},
			wantFindings: map[string]string{
				"APP-002": "fail", "HTTP-001": "review", "HTTP-002": "fail",
				"HTTP-004": "review", "HTTP-008": "review", "DB-001": "fail",
				"DB-007": "review", "REDIS-001": "fail", "JWT-004": "review",
				"MON-001": "fail",
			},
		},
		{
			name: "approved transport exceptions are reviews",
			values: map[string]any{
				"app.Debug":                                          false,
				"app.AllowInsecureProduction":                        true,
				"server.Config.Host":                                 "127.0.0.1",
				"server.Config.Listener.TLSCertFile":                 map[string]bool{"configured": true},
				"server.Config.Listener.TLSCertKeyFile":              map[string]bool{"configured": true},
				"server.Config.Listener.TLSMinVersion":               uint64(0x0301),
				"server.Config.Listener.EnablePrintRoutes":           false,
				"server.Config.Core.ReadTimeout":                     "1s",
				"server.Config.Core.WriteTimeout":                    "1s",
				"server.Config.Core.BodyLimit":                       int64(1024),
				"database.Config.Disable":                            false,
				"database.Config.Driver":                             database.POSTGRES,
				"database.Config.TLS":                                database.TLSDisabled,
				"redis.Config.Disable":                               false,
				"redis.Config.TLS":                                   false,
				"filter.Config.CookieConfig.DisableStoreTokenCookie": true,
				"monitoring.Config.DisableMonitoringEndpoints":       false,
				"monitoring.Config.MonitoringAllowedIPs":             []any{"127.0.0.1"},
			},
			wantFindings: map[string]string{
				"APP-002": "pass", "APP-003": "review", "HTTP-001": "pass",
				"HTTP-002": "pass", "HTTP-003": "fail", "DB-001": "review",
				"REDIS-001": "review", "MON-001": "review",
			},
			wantNoFinding: []string{"HTTP-004", "HTTP-008", "JWT-004"},
		},
		{
			name: "safe settings pass and SQLite TLS is not applicable",
			values: map[string]any{
				"app.Debug":                                          false,
				"server.Config.Host":                                 "127.0.0.1",
				"server.Config.Listener.TLSCertFile":                 map[string]bool{"configured": true},
				"server.Config.Listener.TLSCertKeyFile":              map[string]bool{"configured": true},
				"server.Config.Listener.TLSMinVersion":               uint64(0x0303),
				"server.Config.Core.ReadTimeout":                     "1s",
				"server.Config.Core.WriteTimeout":                    "1s",
				"server.Config.Core.BodyLimit":                       int64(1024),
				"database.Config.Disable":                            false,
				"database.Config.Driver":                             database.SQLITE,
				"redis.Config.Disable":                               false,
				"redis.Config.TLS":                                   true,
				"filter.Config.CookieConfig.DisableStoreTokenCookie": false,
				"filter.Config.CookieConfig.CookieSecure":            true,
				"filter.Config.CookieConfig.CookieHTTPOnly":          true,
				"monitoring.Config.DisableMonitoringEndpoints":       true,
			},
			wantFindings: map[string]string{
				"APP-002": "pass", "HTTP-001": "pass", "HTTP-002": "pass",
				"HTTP-003": "pass", "DB-001": "not_applicable", "REDIS-001": "pass",
			},
			wantNoFinding: []string{"APP-003", "HTTP-004", "HTTP-008", "JWT-004", "MON-001"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inventory := make([]ConfigField, 0, len(test.values))
			for path, value := range test.values {
				inventory = append(inventory, ConfigField{Path: path, Value: value})
			}
			report := Report{Profile: "production", Inventory: inventory}
			assess(&report)

			findings := make(map[string]string, len(report.Findings))
			for _, finding := range report.Findings {
				findings[finding.ID] = finding.Status
			}
			for id, want := range test.wantFindings {
				if got := findings[id]; got != want {
					t.Errorf("finding %s status = %q, want %q", id, got, want)
				}
			}
			for _, id := range test.wantNoFinding {
				if _, exists := findings[id]; exists {
					t.Errorf("unexpected finding %s with status %q", id, findings[id])
				}
			}
		})
	}
}
