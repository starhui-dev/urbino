package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirRejectsMigrationVersionGap(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"0001_first.sql", "0003_third.sql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadDir(dir); err == nil {
		t.Fatal("migration version gap accepted")
	}
}
func TestLoadDirRejectsZeroMigrationVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0000_zero.sql"), []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDir(dir); err == nil {
		t.Fatal("zero migration version accepted")
	}
}

func TestLoadDirRejectsUnrecognizedSQLFilename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0001_first.sql"), []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-a-migration.sql"), []byte("SELECT 2;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDir(dir); err == nil {
		t.Fatal("unrecognized SQL migration filename accepted")
	}
}
