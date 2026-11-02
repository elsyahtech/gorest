package monitoring

import (
	"encoding/json"
	"net"
	"testing"
)

func TestMonitoringConfigJSONUsesCurrentFieldNames(t *testing.T) {
	var config Config
	if err := json.Unmarshal([]byte(`{
		"monitoring_allowed_ips":["192.0.2.10"],
		"enable_http_metrics_monitoring":true
	}`), &config); err != nil {
		t.Fatalf("unmarshal monitoring config: %v", err)
	}

	if len(config.MonitoringAllowedIPs) != 1 || config.MonitoringAllowedIPs[0] != "192.0.2.10" {
		t.Errorf("MonitoringAllowedIPs = %#v, want [192.0.2.10]", config.MonitoringAllowedIPs)
	}
	if !config.EnableHTTPMetricsMonitoring {
		t.Error("EnableHTTPMetricsMonitoring = false, want true")
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal monitoring config: %v", err)
	}
	if got, want := string(encoded), `{"monitoring_allowed_ips":["192.0.2.10"],"disable_monitoring_endpoints":false,"enable_http_metrics_monitoring":true}`; got != want {
		t.Errorf("marshaled monitoring config = %s, want %s", got, want)
	}
}

func TestMonitoringServiceDefaults(t *testing.T) {
	service, err := New(Run())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if !service.EndpointsEnabled() {
		t.Fatal("monitoring endpoints are disabled by default")
	}
	if service.AllowsPeer(net.ParseIP("127.0.0.1")) {
		t.Fatal("default allowlist unexpectedly allows loopback access")
	}
}

func TestMonitoringServiceUsesConfiguredEndpointsAndAllowlist(t *testing.T) {
	service, err := New(Config{
		MonitoringAllowedIPs:       []string{"192.0.2.10"},
		DisableMonitoringEndpoints: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if service.EndpointsEnabled() {
		t.Fatal("monitoring endpoints are enabled despite DisableMonitoringEndpoints")
	}
	if !service.AllowsPeer(net.ParseIP("192.0.2.10")) {
		t.Fatal("configured allowlist does not allow its peer IP")
	}
	if service.AllowsPeer(net.ParseIP("192.0.2.11")) {
		t.Fatal("configured allowlist allows an unlisted peer IP")
	}
}
