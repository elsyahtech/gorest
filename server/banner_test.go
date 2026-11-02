package server

import "testing"

func TestGorestVersion(t *testing.T) {
	if got := gorestVersion(); got == "" {
		t.Fatal("gorestVersion() returned an empty version")
	}
}
