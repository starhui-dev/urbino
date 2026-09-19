// Package contracts_test contains the stage 01 black-box contract tests
// (P01-T01..P01-T06 of checklists/test-matrix.csv).
//
// The tests observe exported behaviour of internal/domain and internal/config
// plus the shipped api/admin.openapi.yaml document. They never modify
// production code, never contact a network or database, use no real secrets,
// and are safe for `go test -count=2`. Genuine failures against the approved
// contract are kept, not weakened.
package contracts_test

import (
	"os"
	"path/filepath"
	"testing"

	"example.com/urbino/internal/testutil"
)

// findRepoRoot walks up from the test directory to the module root.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

// writeConfigFile writes one temporary configuration file with private
// permissions and returns its path.
func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	return testutil.WriteConfig(t, t.TempDir(), content)
}
