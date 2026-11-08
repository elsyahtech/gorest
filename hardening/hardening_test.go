package hardening

import (
	"encoding/json"
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
