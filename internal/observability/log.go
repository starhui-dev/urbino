// Package observability contains the small logging boundary shared by the
// application. Product events and security policy are added in later stages.
package observability

import (
	"log/slog"
	"os"
	"strings"
)

// NewJSONLogger returns a JSON logger with a conservative secret-key redactor.
// Values are still required to be identifiers or summaries; this is a last
// boundary, not permission to log request bodies or credentials.
func NewJSONLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		ReplaceAttr: redactAttribute,
	}))
}

func redactAttribute(_ []string, attr slog.Attr) slog.Attr {
	key := strings.ToLower(attr.Key)
	switch key {
	case "authorization", "api_key", "apikey", "token", "access_token", "refresh_token", "secret", "password", "cookie", "dsn":
		return slog.String(attr.Key, "[REDACTED]")
	default:
		return attr
	}
}
