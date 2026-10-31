package orm

import (
	"context"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
)

func TestNewContextUsesBackgroundWhenUnset(t *testing.T) {
	query := &ORM{DatabaseConfig: &database.Config{}}
	ctx, cancel := query.newContext()
	defer cancel()

	if ctx.Err() != nil {
		t.Fatalf("new background context error = %v", ctx.Err())
	}
	if _, ok := ctx.Deadline(); ok {
		t.Fatal("context without configured timeout unexpectedly has a deadline")
	}
}

func TestNewContextAddsTimeoutOnlyWithoutExistingDeadline(t *testing.T) {
	query := &ORM{DatabaseConfig: &database.Config{Timeout: time.Second}}
	ctx, cancel := query.newContext()
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("configured timeout did not add a deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > time.Second {
		t.Fatalf("context deadline remaining = %s, want positive and at most one second", remaining)
	}

	parent, parentCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer parentCancel()
	query.Context = parent
	child, childCancel := query.newContext()
	defer childCancel()
	childDeadline, ok := child.Deadline()
	if !ok || !childDeadline.Equal(mustContextDeadline(t, parent)) {
		t.Fatal("newContext replaced the caller's existing deadline")
	}
}

func mustContextDeadline(t *testing.T, ctx context.Context) time.Time {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("context has no deadline")
	}

	return deadline
}
