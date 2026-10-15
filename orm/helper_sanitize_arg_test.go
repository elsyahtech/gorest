package orm

import (
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestSanitizeArgsForOracle(t *testing.T) {
	orm := &ORM{DatabaseConfig: &database.Config{Driver: database.ORACLE}}
	value := true
	args := []any{true, false, &value, (*bool)(nil), "unchanged"}

	got := orm.sanitizeArgs(args)
	want := []any{1, 0, 1, nil, "unchanged"}
	if len(got) != len(want) {
		t.Fatalf("got %d args, want %d", len(got), len(want))
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Errorf("arg %d = %#v, want %#v", idx, got[idx], want[idx])
		}
	}

	if args[0] != true || args[2] != &value {
		t.Fatal("sanitizeArgs modified its input")
	}
}

func TestSanitizeArgsLeavesOtherDriversUnchanged(t *testing.T) {
	orm := &ORM{DatabaseConfig: &database.Config{Driver: database.POSTGRES}}
	args := []any{true, false}

	got := orm.sanitizeArgs(args)
	if len(got) != len(args) || got[0] != true || got[1] != false {
		t.Fatalf("sanitizeArgs changed args for a non-Oracle driver: %#v", got)
	}
	if &got[0] != &args[0] {
		t.Fatal("sanitizeArgs should return the original slice for non-Oracle drivers")
	}
}
