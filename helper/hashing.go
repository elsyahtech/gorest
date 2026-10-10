package helper

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// GenerateBcrypt hashes a raw password using bcrypt.
// Returns a 400 Bad Request if the password is empty or longer than 72 bytes (bcrypt limit),
// and a 500 Internal Server Error if hashing fails.
func GenerateBcrypt(rawPassword string) (res string, msg string, code int, err error) {
	if rawPassword == "" {
		return "", "Password must not be empty",
			http.StatusBadRequest,
			errors.New("failed to generate bcrypt hash: password cannot be empty")
	}

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(rawPassword), bcryptCost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return "", "Password must not be longer than 72 bytes",
				http.StatusBadRequest,
				fmt.Errorf("failed to generate bcrypt hash: %w", err)
		}

		return "", "Failed to process password",
			http.StatusInternalServerError,
			fmt.Errorf("failed to generate bcrypt hash: %w", err)
	}

	return string(hashBytes), "", http.StatusOK, nil
}

// GenerateMD5 returns the hex-encoded MD5 digest of the given string.
// WARNING: MD5 is fast and cryptographically broken. Do NOT use it to store passwords;
// use GenerateBcrypt or GenerateArgon instead. Only use it for checksums or legacy compatibility.
// Returns a 400 Bad Request if the input is empty.
func GenerateMD5(rawPassword string) (res string, msg string, code int, err error) {
	if rawPassword == "" {
		return "", "Data must not be empty",
			http.StatusBadRequest,
			errors.New("failed to generate md5 hash: data cannot be empty")
	}

	sum := md5.Sum([]byte(rawPassword))

	return hex.EncodeToString(sum[:]), "", http.StatusOK, nil
}

// GenerateSHA512 returns the hex-encoded SHA-512 digest of the given string.
// WARNING: plain SHA-512 is unsalted and too fast for password storage. Do NOT use it to store passwords;
// use GenerateBcrypt or GenerateArgon instead. Only use it for checksums or integrity checks.
// Returns a 400 Bad Request if the input is empty.
func GenerateSHA512(rawPassword string) (res string, msg string, code int, err error) {
	if rawPassword == "" {
		return "", "Data must not be empty",
			http.StatusBadRequest,
			errors.New("failed to generate sha512 hash: data cannot be empty")
	}

	sum := sha512.Sum512([]byte(rawPassword))

	return hex.EncodeToString(sum[:]), "", http.StatusOK, nil
}

const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024 // 64 MiB
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// GenerateArgon hashes a raw password using Argon2id and returns it in the standard PHC string format:
// $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
// A cryptographically secure random salt is generated for every call and embedded in the result.
// Returns a 400 Bad Request if the password is empty, and a 500 Internal Server Error if salt generation fails.
func GenerateArgon(rawPassword string) (res string, msg string, code int, err error) {
	if rawPassword == "" {
		return "", "Password must not be empty",
			http.StatusBadRequest,
			errors.New("failed to generate argon2id hash: password cannot be empty")
	}

	saltBytes := make([]byte, argonSaltLen)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "Check access to /dev/urandom (or getrandom syscall) in the runtime environment",
			http.StatusInternalServerError,
			fmt.Errorf("failed to generate argon2id salt: %w", err)
	}

	hash := argon2.IDKey([]byte(rawPassword), saltBytes, argonTime, argonMemory, argonThreads, argonKeyLen)

	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(saltBytes),
		base64.RawStdEncoding.EncodeToString(hash),
	)

	return encoded, "", http.StatusOK, nil
}

const msgInvalidCredentials = "Invalid credentials"

// VerifyBcrypt checks a raw password against a bcrypt hash.
// Returns false with a 401 Unauthorized if the password does not match (err is nil in this case).
// Returns a 400 Bad Request if an input is empty, and a 500 Internal Server Error if the stored hash is malformed.
func VerifyBcrypt(rawPassword, hash string) (res bool, msg string, code int, err error) {
	if rawPassword == "" || hash == "" {
		return false, "Password must not be empty",
			http.StatusBadRequest,
			errors.New("failed to verify bcrypt hash: password and hash cannot be empty")
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(rawPassword))
	if err != nil {
		// Wrong password, or password longer than 72 bytes: treat both as invalid credentials.
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return false, msgInvalidCredentials, http.StatusUnauthorized, nil
		}

		// Stored hash is malformed or has an invalid cost: server-side data problem.
		return false, "Failed to process password",
			http.StatusInternalServerError,
			fmt.Errorf("failed to verify bcrypt hash: %w", err)
	}

	return true, "", http.StatusOK, nil
}

// VerifyMD5 checks a raw string against a hex-encoded MD5 digest using a constant-time comparison.
// WARNING: MD5 is broken and must not be used for passwords. Use it only for checksums or legacy compatibility.
// Returns false with a 401 Unauthorized if the digest does not match (err is nil in this case).
func VerifyMD5(rawPassword, hash string) (res bool, msg string, code int, err error) {
	if hash == "" {
		return false, "Hash must not be empty",
			http.StatusBadRequest,
			errors.New("failed to verify md5 hash: hash cannot be empty")
	}

	computed, msg, code, err := GenerateMD5(rawPassword)
	if err != nil {
		return false, msg, code, err
	}

	if subtle.ConstantTimeCompare([]byte(computed), []byte(strings.ToLower(hash))) != 1 {
		return false, msgInvalidCredentials, http.StatusUnauthorized, nil
	}

	return true, "", http.StatusOK, nil
}

// VerifySHA512 checks a raw string against a hex-encoded SHA-512 digest using a constant-time comparison.
// WARNING: plain SHA-512 is unsalted and too fast for passwords. Use it only for checksums or integrity checks.
// Returns false with a 401 Unauthorized if the digest does not match (err is nil in this case).
func VerifySHA512(rawPassword, hash string) (res bool, msg string, code int, err error) {
	if hash == "" {
		return false, "Hash must not be empty",
			http.StatusBadRequest,
			errors.New("failed to verify sha512 hash: hash cannot be empty")
	}

	computed, msg, code, err := GenerateSHA512(rawPassword)
	if err != nil {
		return false, msg, code, err
	}

	if subtle.ConstantTimeCompare([]byte(computed), []byte(strings.ToLower(hash))) != 1 {
		return false, msgInvalidCredentials, http.StatusUnauthorized, nil
	}

	return true, "", http.StatusOK, nil
}

// Policy for hashes accepted by VerifyArgon. Keep in sync with GenerateArgon.
const (
	argonMaxMemory  uint32 = 64 * 1024 // never accept more than the current memory cost
	argonMinMemory  uint32 = 8 * 1024
	argonMaxTime    uint32 = 6
	argonMaxThreads uint8  = 8
	argonDigestLen         = argonKeyLen // 32
)

// parseArgonUint parses "<key>=<digits>" strictly and returns the numeric value.
func parseArgonUint(field, key string, bitSize int) (uint64, error) {
	prefix := key + "="
	if !strings.HasPrefix(field, prefix) {
		return 0, fmt.Errorf("expected %q", prefix)
	}

	digits := strings.TrimPrefix(field, prefix)
	if digits == "" || (len(digits) > 1 && digits[0] == '0') {
		return 0, fmt.Errorf("invalid value for %s", key) // empty or leading zeros
	}

	return strconv.ParseUint(digits, 10, bitSize) // rejects +, -, spaces, trailing chars
}

// VerifyArgon checks a raw password against an Argon2id hash in PHC string format
// ($argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>), as produced by GenerateArgon.
// The encoded hash is parsed strictly and its parameters are validated against a fixed policy
// BEFORE any computation, so a corrupted or tampered hash cannot trigger heavy resource usage.
// Returns false with a 401 Unauthorized if the password does not match (err is nil in this case).
// Returns a 400 Bad Request if an input is empty, and a 500 Internal Server Error if the stored hash is invalid.
func VerifyArgon(rawPassword, encodedHash string) (res bool, msg string, code int, err error) {
	if rawPassword == "" || encodedHash == "" {
		return false, "Password must not be empty",
			http.StatusBadRequest,
			errors.New("failed to verify argon2id hash: password and hash cannot be empty")
	}

	fail := func(cause error) (bool, string, int, error) {
		return false, "Failed to process password",
			http.StatusInternalServerError,
			fmt.Errorf("failed to verify argon2id hash: %w", cause)
	}

	// Expected: "", "argon2id", "v=19", "m=65536,t=3,p=4", "<salt>", "<hash>"
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return fail(errors.New("invalid hash format"))
	}

	// Version must be exactly "v=19".
	if parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return fail(fmt.Errorf("unsupported argon2 version: %q", parts[2]))
	}

	// Parameters must be exactly three fields in order: m, t, p (no duplicates, no extras).
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return fail(errors.New("invalid parameters format"))
	}

	m, err := parseArgonUint(params[0], "m", 32)
	if err != nil {
		return fail(fmt.Errorf("invalid memory: %w", err))
	}
	t, err := parseArgonUint(params[1], "t", 32)
	if err != nil {
		return fail(fmt.Errorf("invalid time: %w", err))
	}
	p, err := parseArgonUint(params[2], "p", 8)
	if err != nil {
		return fail(fmt.Errorf("invalid threads: %w", err))
	}

	memory, iterations, threads := uint32(m), uint32(t), uint8(p)

	// Policy limits (reject zero, too small, and too large).
	if iterations == 0 || iterations > argonMaxTime {
		return fail(fmt.Errorf("time out of range: %d", iterations))
	}
	if threads == 0 || threads > argonMaxThreads {
		return fail(fmt.Errorf("threads out of range: %d", threads))
	}
	if memory < argonMinMemory || memory > argonMaxMemory || memory < 8*uint32(threads) {
		return fail(fmt.Errorf("memory out of range: %d", memory))
	}

	// Strict base64 decoding, with fixed expected lengths.
	b64 := base64.RawStdEncoding.Strict()

	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) != argonSaltLen {
		return fail(errors.New("invalid salt"))
	}

	expected, err := b64.DecodeString(parts[5])
	if err != nil || len(expected) != int(argonDigestLen) {
		return fail(errors.New("invalid digest"))
	}

	computed := argon2.IDKey([]byte(rawPassword), salt, iterations, memory, threads, argonDigestLen)

	if subtle.ConstantTimeCompare(computed, expected) != 1 {
		return false, msgInvalidCredentials, http.StatusUnauthorized, nil
	}

	return true, "", http.StatusOK, nil
}
