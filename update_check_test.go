package gorest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckForUpdate(t *testing.T) {
	tests := []struct {
		name          string
		installed     string
		status        int
		body          string
		wantVersion   string
		wantErr       bool
		wantNoRelease bool
	}{
		{
			name:        "newer release available",
			installed:   "v1.1.12",
			status:      http.StatusOK,
			body:        `{"tag_name":"v1.2.0"}`,
			wantVersion: "v1.2.0",
		},
		{
			name:      "current release is latest",
			installed: "v1.2.0",
			status:    http.StatusOK,
			body:      `{"tag_name":"v1.2.0"}`,
		},
		{
			name:      "no newer release",
			installed: "v1.3.0",
			status:    http.StatusOK,
			body:      `{"tag_name":"v1.2.0"}`,
		},
		{
			name:      "development build is not comparable",
			installed: "development (untagged)",
			status:    http.StatusInternalServerError,
			body:      "",
		},
		{
			name:      "release request failure",
			installed: "v1.1.12",
			status:    http.StatusServiceUnavailable,
			body:      `{}`,
			wantErr:   true,
		},
		{
			name:          "no published latest release",
			installed:     "v1.1.18",
			status:        http.StatusNotFound,
			body:          `{"message":"Not Found"}`,
			wantNoRelease: true,
		},
		{
			name:      "invalid release version",
			installed: "v1.1.12",
			status:    http.StatusOK,
			body:      `{"tag_name":"latest"}`,
			wantErr:   true,
		},
		{
			name:      "malformed release response",
			installed: "v1.1.12",
			status:    http.StatusOK,
			body:      `{"tag_name":`,
			wantErr:   true,
		},
		{
			name:      "release response exceeds size limit",
			installed: "v1.1.12",
			status:    http.StatusOK,
			body:      strings.Repeat("x", maxReleaseResponse+1),
			wantErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requestSeen atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requestSeen.Store(true)
				if request.Method != http.MethodGet {
					t.Errorf("request method = %q, want GET", request.Method)
				}
				if request.Header.Get("User-Agent") != "gorest-update-check" {
					t.Errorf("User-Agent = %q, want gorest-update-check", request.Header.Get("User-Agent"))
				}

				writer.WriteHeader(test.status)
				_, _ = fmt.Fprint(writer, test.body)
			}))
			defer server.Close()

			notice, err := checkForUpdate(context.Background(), test.installed, server.Client(), server.URL)
			if test.wantNoRelease {
				if !errors.Is(err, errNoPublishedGORESTRelease) {
					t.Fatalf("checkForUpdate() error = %v, want no-published-release result", err)
				}
			} else if (err != nil) != test.wantErr {
				t.Fatalf("checkForUpdate() error = %v, wantErr %t", err, test.wantErr)
			}
			if test.wantVersion != "" {
				if notice == nil {
					t.Fatal("checkForUpdate() returned no update notice")
				}
				if notice.LatestVersion != test.wantVersion {
					t.Errorf("latest version = %q, want %q", notice.LatestVersion, test.wantVersion)
				}
				if notice.InstalledVersion != test.installed {
					t.Errorf("installed version = %q, want %q", notice.InstalledVersion, test.installed)
				}
			} else if notice != nil {
				t.Errorf("checkForUpdate() notice = %#v, want nil", notice)
			}
			if test.installed == "development (untagged)" && requestSeen.Load() {
				t.Error("development build unexpectedly requested release information")
			}
		})
	}
}

func TestCheckForUpdateTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	notice, err := checkForUpdate(ctx, "v1.1.12", server.Client(), server.URL)
	if err == nil {
		t.Fatal("checkForUpdate() error = nil, want timeout error")
	}
	if notice != nil {
		t.Fatalf("checkForUpdate() notice = %#v, want nil", notice)
	}
}

func TestUpdateCheckDefaultAndHelpers(t *testing.T) {
	if !updateChecksEnabled(nil) {
		t.Fatal("nil CheckForUpdates should enable update notifications")
	}

	if !updateChecksEnabled(configDefault(Config{}).CheckForUpdates) {
		t.Fatal("configDefault() did not enable update notifications by default")
	}

	if !updateChecksEnabled(EnableAutoUpdate) {
		t.Fatal("EnableAutoUpdate is disabled")
	}
	if updateChecksEnabled(DisableAutoUpdate) {
		t.Fatal("DisableAutoUpdate is enabled")
	}

	config := configDefault(Config{CheckForUpdates: DisableAutoUpdate})
	if updateChecksEnabled(config.CheckForUpdates) {
		t.Fatal("configDefault() discarded the explicit opt-out")
	}
}
