package securitylog

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggerDoesNotSerializeErrorText(t *testing.T) {
	var output bytes.Buffer
	logger := New(slog.New(slog.NewJSONHandler(&output, nil)))
	logger.Error("request_failed", errors.New("api_key=secret-value"))
	if strings.Contains(output.String(), "secret-value") {
		t.Fatalf("log leaked error text: %s", output.String())
	}
}
