package postgres

import (
	"errors"
	"strings"
	"testing"
)

func TestSafeErrorMessageDoesNotLeakDSNOrDriverText(t *testing.T) {
	secret := "postgres://app:super-secret@example.invalid/urbino"
	err := &Error{Kind: ErrorQuery, err: errors.New("driver rejected " + secret)}
	message := SafeErrorMessage(err)
	if message != "database operation failed" {
		t.Fatalf("safe message = %q", message)
	}
	if strings.Contains(message, "super-secret") || strings.Contains(message, secret) || strings.Contains(message, "driver rejected") {
		t.Fatalf("safe message leaked driver detail: %q", message)
	}
}

func TestOpenRejectsBlankDSNWithoutDetail(t *testing.T) {
	_, err := Open(t.Context(), Config{DSN: "   "})
	if err == nil {
		t.Fatal("blank DSN accepted")
	}
	if got := SafeErrorMessage(err); got != "database configuration is invalid" {
		t.Fatalf("safe message = %q", got)
	}
}
