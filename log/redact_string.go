//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import "unicode"

func redactString(valueParam string) string {
	value := cookieHeaderPattern.ReplaceAllString(valueParam, "${1}"+redactedValue)
	value = sensitiveAssignmentPattern.ReplaceAllString(value, "${1}"+redactedValue)
	value = bearerPattern.ReplaceAllString(value, "${1} "+redactedValue)
	value = jwtPattern.ReplaceAllString(value, redactedValue)
	value = emailPattern.ReplaceAllString(value, redactedValue)

	return phonePattern.ReplaceAllStringFunc(value, func(candidate string) string {
		digitCount := 0

		for _, char := range candidate {
			if unicode.IsDigit(char) {
				digitCount++
			}
		}

		if digitCount >= 10 && digitCount <= 15 {
			return redactedValue
		}

		return candidate
	})
}
