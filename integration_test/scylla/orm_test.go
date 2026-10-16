//go:build integration

package scylla_test

import (
	"testing"

	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestORMCRUD(t *testing.T) {
	internaltest.ScyllaCRUD(t)
}
