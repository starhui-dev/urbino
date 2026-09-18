// Package testutil contains small helpers shared by repository tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// WriteConfig writes a private temporary configuration file and returns its
// path. It is intentionally test-only infrastructure and never logs contents.
func WriteConfig(t testing.TB, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "urbino.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
