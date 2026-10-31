//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import (
	"regexp"

	"go.uber.org/zap/zapcore"
)

const redactedValue = "xxxxx"

var (
	cookieHeaderPattern        = regexp.MustCompile(`(?i)(\b(?:cookie|set-cookie)\b\s*[:=]\s*)[^\r\n]+`)
	sensitiveAssignmentPattern = regexp.MustCompile(`(?i)(\b(?:password|passwd|pwd|token|access[_ -]?token|refresh[_ -]?token|secret|credential|authorization|proxy[_ -]?authorization|cookie|set-cookie|session(?:[_ -]?(?:id|key|token))?|api[_ -]?key|private[_ -]?key|client[_ -]?secret|ssn|social[_ -]?security|credit[_ -]?card|card[_ -]?(?:number|no)|cvv|cvc)\b\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;&]+)`) //nolint:revive,lll
	bearerPattern              = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]+`)
	jwtPattern                 = regexp.MustCompile(`\b[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)
	emailPattern               = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	phonePattern               = regexp.MustCompile(`\+?[0-9][0-9(). -]{8,}[0-9]`)
)

type redactingCore struct {
	zapcore.Core
}

func (core redactingCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if core.Enabled(entry.Level) {
		return checked.AddCore(entry, core)
	}

	return checked
}
