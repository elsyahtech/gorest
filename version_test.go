package gorest

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
			name: "release dependency replacement",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: "example.com/application", Version: "v2.0.0"},
				Deps: []*debug.Module{{
					Path:    gorestModulePath,
					Version: "v1.1.0",
					Replace: &debug.Module{Path: gorestModulePath, Version: "v1.1.5"},
				}},
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
			want: developmentGORESTVersion,
		},
		{
			name: "module missing",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: "example.com/application", Version: "v2.0.0"},
			},
			want: developmentGORESTVersion,
		},
		{
			name: "build info missing",
			want: developmentGORESTVersion,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := gorestVersionFromBuildInfo(test.info); got != test.want {
				t.Fatalf("gorestVersionFromBuildInfo() = %q, want %q", got, test.want)
			}
		})
	}

	if got := currentGORESTVersion(); got == "" {
		t.Fatal("currentGORESTVersion() returned an empty version")
	}
}
