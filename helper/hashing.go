package helper

import (
	"crypto/md5" //nolint:revive
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
func GenerateBcrypt(value string) (res string, msg string, code int, err error) {
	if value == "" {
		const message = "Password must not be empty"

		return "", message, http.StatusBadRequest, errors.New("failed to generate bcrypt hash: password cannot be empty")
	}

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(value), bcryptCost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			const message = "Password must not be longer than 72 bytes"

			return "", message, http.StatusBadRequest, fmt.Errorf("failed to generate bcrypt hash: %w", err)
		}

		const message = "Failed to process password"

		return "", message, http.StatusInternalServerError, fmt.Errorf("failed to generate bcrypt hash: %w", err)
	}

	return string(hashBytes), "", http.StatusOK, nil
}

// GenerateMD5 returns the hex-encoded MD5 digest of the given string.
// WARNING: MD5 is fast and cryptographically broken. Do NOT use it to store passwords;
// use GenerateBcrypt or GenerateArgon instead. Only use it for checksums or legacy compatibility.
// Returns a 400 Bad Request if the input is empty.
//
// Deprecated: MD5 is cryptographically broken. Keep this only for legacy
// compatibility; use a modern, purpose-appropriate hash or MAC for new code.
func GenerateMD5(value string) (res string, msg string, code int, err error) {
	if value == "" {
		const message = "Data must not be empty"

		return "", message, http.StatusBadRequest, errors.New("failed to generate md5 hash: data cannot be empty")
	}

	sum := md5.Sum([]byte(value))

	return hex.EncodeToString(sum[:]), "", http.StatusOK, nil
}

// GenerateSHA512 returns the hex-encoded SHA-512 digest of the given string.
// SHA-512 is an unkeyed, unsalted, fast digest. Use it only for non-secret
// checksums or integrity checks where an unkeyed digest is appropriate. It does
// not authenticate data and must never be used to store passwords, password
// reset tokens, API keys, or other secrets. Use GenerateBcrypt or GenerateArgon
// for passwords, and a keyed MAC such as HMAC when authenticity is required.
// Returns a 400 Bad Request if the input is empty.
func GenerateSHA512(value string) (res string, msg string, code int, err error) {
	if value == "" {
		const message = "Data must not be empty"

		return "", message, http.StatusBadRequest, errors.New("failed to generate sha512 hash: data cannot be empty")
	}

	sum := sha512.Sum512([]byte(value))

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
func GenerateArgon(value string) (res string, msg string, code int, err error) {
	if value == "" {
		const message = "Password must not be empty"

		return "", message, http.StatusBadRequest, errors.New("failed to generate argon2id hash: password cannot be empty")
	}

	saltBytes := make([]byte, argonSaltLen)
	if _, err := rand.Read(saltBytes); err != nil {
		const message = "Check access to /dev/urandom (or getrandom syscall) in the runtime environment"

		return "", message, http.StatusInternalServerError, fmt.Errorf("failed to generate argon2id salt: %w", err)
	}

	hash := argon2.IDKey([]byte(value), saltBytes, argonTime, argonMemory, argonThreads, argonKeyLen)

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
func VerifyBcrypt(value, hash string) (res bool, msg string, code int, err error) {
	if value == "" || hash == "" {
		const message = "Password must not be empty"

		return false, message, http.StatusBadRequest, errors.New("failed to verify bcrypt hash: password and hash cannot be empty")
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(value))
	if err != nil {
		// Wrong password, or password longer than 72 bytes: treat both as invalid credentials.
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return false, msgInvalidCredentials, http.StatusUnauthorized, nil
		}

		// Stored hash is malformed or has an invalid cost: server-side data problem.
		const message = "Failed to process password"

		return false, message, http.StatusInternalServerError, fmt.Errorf("failed to verify bcrypt hash: %w", err)
	}

	return true, "", http.StatusOK, nil
}

// VerifyMD5 checks a raw string against a hex-encoded MD5 digest using a constant-time comparison.
// WARNING: MD5 is broken and must not be used for passwords. Use it only for checksums or legacy compatibility.
// Returns false with a 401 Unauthorized if the digest does not match (err is nil in this case).
//
// Deprecated: MD5 is cryptographically broken. Keep this only for legacy
// compatibility; use a modern, purpose-appropriate hash or MAC for new code.
func VerifyMD5(value, hash string) (res bool, msg string, code int, err error) {
	if hash == "" {
		const message = "Hash must not be empty"

		return false, message, http.StatusBadRequest, errors.New("failed to verify md5 hash: hash cannot be empty")
	}

	computed, msg, code, err := GenerateMD5(value)
	if err != nil {
		return false, msg, code, err
	}

	if subtle.ConstantTimeCompare([]byte(computed), []byte(strings.ToLower(hash))) != 1 {
		return false, msgInvalidCredentials, http.StatusUnauthorized, nil
	}

	return true, "", http.StatusOK, nil
}

// VerifySHA512 checks a value against a hex-encoded SHA-512 digest using a
// constant-time comparison. SHA-512 is an unkeyed, unsalted, fast digest; use
// this only for non-secret checksums or integrity checks. It does not
// authenticate data and must not be used to verify passwords or other secrets.
// Returns false with a 401 Unauthorized if the digest does not match (err is nil in this case).
func VerifySHA512(value, hash string) (res bool, msg string, code int, err error) {
	if hash == "" {
		const message = "Hash must not be empty"

		return false, message, http.StatusBadRequest, errors.New("failed to verify sha512 hash: hash cannot be empty")
	}

	computed, msg, code, err := GenerateSHA512(value)
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

	// rejects +, -, spaces, trailing chars
	return strconv.ParseUint(digits, 10, bitSize) //nolint:wrapcheck
}

// VerifyArgon checks a raw password against an Argon2id hash in PHC string format
// ($argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>), as produced by GenerateArgon.
// The encoded hash is parsed strictly and its parameters are validated against a fixed policy
// BEFORE any computation, so a corrupted or tampered hash cannot trigger heavy resource usage.
// Returns false with a 401 Unauthorized if the password does not match (err is nil in this case).
// Returns a 400 Bad Request if an input is empty, and a 500 Internal Server Error if the stored hash is invalid.
func VerifyArgon(value, encodedHash string) (res bool, msg string, code int, err error) {
	if value == "" || encodedHash == "" {
		const message = "Password must not be empty"

		return false, message, http.StatusBadRequest, errors.New("failed to verify argon2id hash: password and hash cannot be empty")
	}

	// Expected: "", "argon2id", "v=19", "m=65536,t=3,p=4", "<salt>", "<hash>"
	parts, emm, tee, pee, err := parts(encodedHash)
	if err != nil {
		return fail(err)
	}

	memory, iterations, threads := uint32(emm), uint32(tee), uint8(pee)

	if err := validation(iterations, threads, memory); err != nil {
		return fail(err)
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

	computed := argon2.IDKey([]byte(value), salt, iterations, memory, threads, argonDigestLen)

	if subtle.ConstantTimeCompare(computed, expected) != 1 {
		return false, msgInvalidCredentials, http.StatusUnauthorized, nil
	}

	return true, "", http.StatusOK, nil
}

func fail(cause error) (bool, string, int, error) {
	return false, "Failed to process password",
		http.StatusInternalServerError,
		fmt.Errorf("failed to verify argon2id hash: %w", cause)
}

func parts(encodedHash string) (
	parts []string, emm uint64, tee uint64, pee uint64, err error,
) {
	parts = strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return nil, 0, 0, 0, errors.New("invalid hash format")
	}

	// Version must be exactly "v=19".
	if parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return nil, 0, 0, 0, fmt.Errorf("unsupported argon2 version: %q", parts[2])
	}

	// Parameters must be exactly three fields in order: m, t, p (no duplicates, no extras).
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return nil, 0, 0, 0, errors.New("invalid parameters format")
	}

	emm, err = parseArgonUint(params[0], "m", 32)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("invalid memory: %w", err)
	}

	tee, err = parseArgonUint(params[1], "t", 32)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("invalid time: %w", err)
	}

	pee, err = parseArgonUint(params[2], "p", 8)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("invalid threads: %w", err)
	}

	return parts, emm, tee, pee, nil
}

func validation(iterations uint32, threads uint8, memory uint32) error {
	// Policy limits (reject zero, too small, and too large).
	if iterations == 0 || iterations > argonMaxTime {
		return fmt.Errorf("time out of range: %d", iterations)
	}

	if threads == 0 || threads > argonMaxThreads {
		return fmt.Errorf("threads out of range: %d", threads)
	}

	if memory < argonMinMemory || memory > argonMaxMemory || memory < 8*uint32(threads) {
		return fmt.Errorf("memory out of range: %d", memory)
	}

	return nil
} //nolint:revive
