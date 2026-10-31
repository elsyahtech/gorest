//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import (
	"reflect"
	"strings"
	"unicode"
)

func redactEntry(key string, item reflect.Value, depth int) any {
	if isSensitiveKey(key) {
		return redactedValue
	}

	return redactValue(item, depth+1)
}

func isSensitiveKey(key string) bool {
	normalizedKey := strings.Map(func(char rune) rune {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			return char
		}

		return -1
	}, strings.ToLower(key))

	for _, marker := range []string{
		"password", "passwd", "pwd", "token", "secret", "credential",
		"authorization", "cookie", "session", "sessionid", "sessionkey", "sessiontoken",
		"apikey", "privatekey", "clientsecret", "ssn", "socialsecurity",
		"creditcard", "cardnumber", "cardno", "cvv", "cvc",
	} {
		if strings.Contains(normalizedKey, marker) {
			return true
		}
	}

	//nolint:revive
	return normalizedKey == "email" ||
		strings.HasSuffix(normalizedKey, "email") ||
		strings.HasSuffix(normalizedKey, "mailaddress") ||
		normalizedKey == "phone" ||
		strings.HasSuffix(normalizedKey, "phone") ||
		normalizedKey == "mobile" ||
		strings.HasSuffix(normalizedKey, "mobile")
}
