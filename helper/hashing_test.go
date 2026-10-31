package helper

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestBcryptHashingAndVerification(t *testing.T) {
	if _, message, code, err := GenerateBcrypt(""); err == nil || message == "" || code != http.StatusBadRequest {
		t.Fatalf("GenerateBcrypt(empty) = (%q, %d, %v), want bad request", message, code, err)
	}
	if _, message, code, err := GenerateBcrypt(strings.Repeat("x", 73)); err == nil ||
		message == "" || code != http.StatusBadRequest {
		t.Fatalf("GenerateBcrypt(long) = (%q, %d, %v), want bad request", message, code, err)
	}

	hash, message, code, err := GenerateBcrypt("correct horse")
	if err != nil || message != "" || code != http.StatusOK {
		t.Fatalf("GenerateBcrypt() = (%q, %d, %v), want success", message, code, err)
	}

	for _, test := range []struct {
		name  string
		value string
		hash  string
		want  bool
		code  int
	}{
		{name: "match", value: "correct horse", hash: hash, want: true, code: http.StatusOK},
		{name: "mismatch", value: "wrong horse", hash: hash, code: http.StatusUnauthorized},
		{name: "missing password", hash: hash, code: http.StatusBadRequest},
		{name: "missing hash", value: "correct horse", code: http.StatusBadRequest},
		{name: "malformed hash", value: "correct horse", hash: "not-a-bcrypt-hash", code: http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			matched, _, gotCode, verifyErr := VerifyBcrypt(test.value, test.hash)
			if matched != test.want || gotCode != test.code {
				t.Fatalf("VerifyBcrypt() = (%t, %d), want (%t, %d)", matched, gotCode, test.want, test.code)
			}
			if (verifyErr != nil) != (test.code == http.StatusInternalServerError || test.code == http.StatusBadRequest) {
				t.Fatalf("VerifyBcrypt() error = %v", verifyErr)
			}
		})
	}
}

func TestLegacyDigestHashingAndVerification(t *testing.T) {
	for _, test := range []struct {
		name     string
		generate func(string) (string, string, int, error)
		verify   func(string, string) (bool, string, int, error)
		want     string
	}{
		{name: "md5", generate: GenerateMD5, verify: VerifyMD5, want: "5d41402abc4b2a76b9719d911017c592"},
		{name: "sha512", generate: GenerateSHA512, verify: VerifySHA512,
			want: "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca7" +
				"2323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043"},
	} {
		t.Run(test.name, func(t *testing.T) {
			hash, _, code, err := test.generate("hello")
			if err != nil || code != http.StatusOK || hash != test.want {
				t.Fatalf("generate() = (%q, %d, %v), want digest %q", hash, code, err, test.want)
			}
			for _, verify := range []struct {
				name  string
				value string
				hash  string
				want  bool
				code  int
			}{
				{name: "match", value: "hello", hash: strings.ToUpper(hash), want: true, code: http.StatusOK},
				{name: "mismatch", value: "bye", hash: hash, code: http.StatusUnauthorized},
				{name: "empty hash", value: "hello", code: http.StatusBadRequest},
				{name: "empty value", hash: hash, code: http.StatusBadRequest},
			} {
				t.Run(verify.name, func(t *testing.T) {
					matched, _, gotCode, verifyErr := test.verify(verify.value, verify.hash)
					if matched != verify.want || gotCode != verify.code {
						t.Fatalf("verify() = (%t, %d), want (%t, %d)", matched, gotCode, verify.want, verify.code)
					}
					if (verifyErr != nil) != (verify.code == http.StatusBadRequest) {
						t.Fatalf("verify() error = %v", verifyErr)
					}
				})
			}
			if _, message, code, err := test.generate(""); err == nil || message == "" || code != http.StatusBadRequest {
				t.Fatalf("generate(empty) = (%q, %d, %v), want bad request", message, code, err)
			}
		})
	}
}

func TestArgonHashingAndStrictVerification(t *testing.T) {
	if _, message, code, err := GenerateArgon(""); err == nil || message == "" || code != http.StatusBadRequest {
		t.Fatalf("GenerateArgon(empty) = (%q, %d, %v), want bad request", message, code, err)
	}
	hash, message, code, err := GenerateArgon("correct horse")
	if err != nil || message != "" || code != http.StatusOK {
		t.Fatalf("GenerateArgon() = (%q, %d, %v), want success", message, code, err)
	}
	if matched, _, gotCode, verifyErr := VerifyArgon("correct horse", hash); !matched ||
		gotCode != http.StatusOK || verifyErr != nil {
		t.Fatalf("VerifyArgon(valid) = (%t, %d, %v), want success", matched, gotCode, verifyErr)
	}
	if matched, _, gotCode, verifyErr := VerifyArgon("wrong horse", hash); matched ||
		gotCode != http.StatusUnauthorized || verifyErr != nil {
		t.Fatalf("VerifyArgon(mismatch) = (%t, %d, %v), want unauthorized", matched, gotCode, verifyErr)
	}
	if _, _, gotCode, verifyErr := VerifyArgon("", hash); verifyErr == nil || gotCode != http.StatusBadRequest {
		t.Fatalf("VerifyArgon(empty) = (%d, %v), want bad request", gotCode, verifyErr)
	}

	for _, malformed := range []string{
		"not-a-phc-string",
		"$argon2id$v=18$m=65536,t=3,p=4$c2FsdA$ZGlnaWVzdA",
		"$argon2id$v=19$m=8,t=3,p=4$c2FsdA$ZGlnaWVzdA",
		"$argon2id$v=19$m=65536,t=3,p=4$%%%$ZGlnaWVzdA",
	} {
		if _, _, gotCode, verifyErr := VerifyArgon("correct horse", malformed); verifyErr == nil ||
			gotCode != http.StatusInternalServerError {
			t.Errorf("VerifyArgon(%q) = (%d, %v), want invalid-hash error", malformed, gotCode, verifyErr)
		}
	}
	if _, _, gotCode, verifyErr := VerifyArgon("password", "$argon2id$v=19$m=8192,t=1,p=1$%%%$ZGlnaWVzdA"); verifyErr == nil ||
		gotCode != http.StatusInternalServerError {
		t.Errorf("VerifyArgon(invalid base64 salt) = (%d, %v), want invalid-hash error", gotCode, verifyErr)
	}

	fixtureSalt := []byte("0123456789abcdef")
	fixtureDigest := argon2.IDKey([]byte("fixture password"), fixtureSalt, 1, argonMinMemory, 1, argonDigestLen)
	fixture := "$argon2id$v=19$m=8192,t=1,p=1$" +
		base64.RawStdEncoding.EncodeToString(fixtureSalt) + "$" +
		base64.RawStdEncoding.EncodeToString(fixtureDigest)
	if matched, _, gotCode, verifyErr := VerifyArgon("fixture password", fixture); !matched ||
		gotCode != http.StatusOK || verifyErr != nil {
		t.Fatalf("VerifyArgon(low-cost valid fixture) = (%t, %d, %v), want success", matched, gotCode, verifyErr)
	}
}

func TestArgonParameterParserAndPolicy(t *testing.T) {
	for _, field := range []string{"t=3", "m=", "m=01", "m=-1", "m=4294967296"} {
		if _, err := parseArgonUint(field, "m", 32); err == nil {
			t.Errorf("parseArgonUint(%q) unexpectedly succeeded", field)
		}
	}
	if value, err := parseArgonUint("m=65536", "m", 32); err != nil || value != 65536 {
		t.Fatalf("parseArgonUint() = (%d, %v), want (65536, nil)", value, err)
	}

	for _, encoded := range []string{
		"$argon2id$v=19$m=1,t=1,p=1$salt$digest",
		"$argon2id$v=19$m=8192,t=0,p=1$salt$digest",
		"$argon2id$v=19$m=8192,t=1,p=0$salt$digest",
		"$argon2id$v=19$m=8192,t=1,p=2$salt$digest",
		"$argon2id$v=19$m=8192,t=1,p=1,extra=2$salt$digest",
	} {
		if _, _, code, err := VerifyArgon("password", encoded); err == nil || code != http.StatusInternalServerError {
			t.Errorf("VerifyArgon(%q) = (%d, %v), want invalid-hash error", encoded, code, err)
		}
	}

	if matched, message, code, err := fail(errors.New("fixture")); matched || message == "" ||
		code != http.StatusInternalServerError || err == nil {
		t.Errorf("fail() = (%t, %q, %d, %v), want internal error", matched, message, code, err)
	}
}
