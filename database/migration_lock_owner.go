package database

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func newMigrationLockOwner() (string, error) {
	var bytes [16]byte

	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("%w", err)
	}

	return hex.EncodeToString(bytes[:]), nil
}
