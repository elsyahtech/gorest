package monitoring

import (
	"net"
	"testing"
)

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
		HTTPMetricsAllowedIPs:      []string{"192.0.2.10"},
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
