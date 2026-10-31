package gorest

import (
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/filter"
)

func TestProductionFilterCookieWarning(t *testing.T) {
	tests := []struct {
		config     *filter.Config
		name       string
		wantReason string
		wantWarn   bool
	}{
		{name: "filter disabled"},
		{
			name: "secure cookies",
			config: &filter.Config{
				CookieSecure: true,
			},
		},
		{
			name:       "insecure cookies",
			config:     &filter.Config{},
			wantWarn:   true,
			wantReason: "may be sent over unencrypted HTTP",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			warning, shouldWarn := productionFilterCookieWarning(test.config)
			if shouldWarn != test.wantWarn {
				t.Fatalf("shouldWarn = %t, want %t (warning: %q)", shouldWarn, test.wantWarn, warning)
			}
			if test.wantReason != "" && !strings.Contains(warning, test.wantReason) {
				t.Errorf("warning = %q, want it to contain %q", warning, test.wantReason)
			}
		})
	}
}
