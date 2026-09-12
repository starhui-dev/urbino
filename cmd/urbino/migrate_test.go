package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	urbinoConfig "github.com/starhui-dev/urbino/internal/config"
)

func TestMigrateRequiresExplicitTarget(t *testing.T) {
	for _, args := range [][]string{{}, {"--dsn-file", "missing"}, {"--dsn-file", "missing", "--database", "urbino_test", "--environment", "staging"}} {
		var out, errOut bytes.Buffer
		if err := migrateCommand(args, &out, &errOut); err == nil || out.Len() != 0 {
			t.Fatalf("migrate accepted incomplete target: %v", args)
		}
	}
}

func TestMigrationTargetRejectsOverrides(t *testing.T) {
	const base = "postgres://migrator:synthetic@127.0.0.1:5432/urbino_test"
	if err := validateMigrationTarget(base, "urbino_test"); err != nil {
		t.Fatal(err)
	}
	for _, dsn := range []string{base + "?dbname=other", base + "?host=other", base + "?service=other", base + "?passfile=other", "postgres://migrator:synthetic@127.0.0.1/production", "invalid synthetic-secret"} {
		if err := validateMigrationTarget(dsn, "urbino_test"); err == nil || strings.Contains(err.Error(), "synthetic") {
			t.Fatal("invalid target accepted or secret leaked")
		}
	}
}

func TestDatabaseSecretReadIsBoundedAndSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dsn")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxDSNBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDSNFile(path); err == nil {
		t.Fatal("oversized secret accepted")
	}
	if _, err := readDSNFile(filepath.Join(t.TempDir(), "synthetic-secret")); err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal("missing file leaked path")
	}
	if _, err := databaseURL(urbinoConfig.Database{URL: "synthetic", URLFile: path}); err == nil {
		t.Fatal("ambiguous connection config accepted")
	}
}
