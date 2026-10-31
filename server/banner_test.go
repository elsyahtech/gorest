package server

import (
	"runtime/debug"
	"testing"
)

func TestGorestVersionFromBuildInfo(t *testing.T) {
	tests := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{
			name: "release dependency",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: "example.com/application", Version: "v2.0.0"},
				Deps: []*debug.Module{{Path: gorestModulePath, Version: "v1.1.5"}},
			},
			want: "v1.1.5",
		},
		{
			name: "release as main module",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: gorestModulePath, Version: "v1.1.5"},
			},
			want: "v1.1.5",
		},
		{
			name: "development build",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: gorestModulePath, Version: "(devel)"},
			},
			want: "development (untagged)",
		},
		{
			name: "module missing",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: "example.com/application", Version: "v2.0.0"},
			},
			want: "development (untagged)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := gorestVersionFromBuildInfo(test.info); got != test.want {
				t.Fatalf("gorestVersionFromBuildInfo() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGorestVersion(t *testing.T) {
	if got := gorestVersion(); got == "" {
		t.Fatal("gorestVersion() returned an empty version")
	}
}
