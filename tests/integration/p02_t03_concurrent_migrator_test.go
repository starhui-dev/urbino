package integration_test

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"testing"

	"example.com/urbino/tests/integration"
)

// migrateVersion parses the numeric version prefix of a migration file name
// (same shape as the production loader accepts, e.g. 0001_persistence.sql).
func migrateVersion(t *testing.T, fileName string) int64 {
	t.Helper()
	match := regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`).FindStringSubmatch(fileName)
	if match == nil {
		t.Fatalf("migration file %q does not match the production naming contract", fileName)
	}
	version, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		t.Fatalf("parse migration version from %q: %v", fileName, err)
	}
	return version
}

// TestP02T03ConcurrentMigratorsSerialize proves that racing migrators do not
// corrupt the schema: four `urbino migrate` processes are started against the
// same empty test database at the same moment, and the advisory lock plus
// history bookkeeping must let every process exit 0 while the migration is
// applied exactly once. (Timing-based proof that processes overlapped is not
// attempted; the observable contract is all-success plus single application.)
func TestP02T03ConcurrentMigratorsSerialize(t *testing.T) {
	e := requireRealPG(t)
	if len(e.migrations) == 0 {
		t.Fatal("no migration files found (phase 02 migrations missing)")
	}
	_, rawDSN, conn := e.scratchDatabase(t, "_race")
	const historyTable = "urbino_schema_migrations"

	type migrateResult struct {
		exit int
		out  string
	}
	const racers = 4
	results := make([]migrateResult, racers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			exit, out, err := e.migrateDSN(t.Context(), rawDSN)
			if err != nil {
				results[i] = migrateResult{exit: -1, out: err.Error()}
				return
			}
			results[i] = migrateResult{exit: exit, out: out}
		}(i)
	}
	close(start)
	wg.Wait()

	for i, r := range results {
		if r.exit != 0 {
			t.Fatalf("concurrent migrator %d exited %d (all racers must succeed): %s", i, r.exit, r.out)
		}
	}

	var applied int64
	if err := conn.QueryRow(t.Context(), "SELECT count(*) FROM "+historyTable).Scan(&applied); err != nil {
		t.Fatalf("read migration history: %s", integration.RedactDSN(err.Error()))
	}
	expected := int64(0)
	for _, fileName := range e.migrations {
		migrateVersion(t, fileName)
		expected++
	}
	if applied != expected {
		t.Fatalf("migration history has %d rows after the race, want exactly %d (one row per migration)", applied, expected)
	}
	for _, fileName := range e.migrations {
		version := migrateVersion(t, fileName)
		var count int64
		if err := conn.QueryRow(t.Context(),
			fmt.Sprintf("SELECT count(*) FROM %s WHERE version = $1", historyTable), version).Scan(&count); err != nil {
			t.Fatalf("read history for version %d: %s", version, integration.RedactDSN(err.Error()))
		}
		if count != 1 {
			t.Fatalf("migration %s (version %d) was applied %d times by concurrent migrators, want exactly 1", fileName, version, count)
		}
	}

	// A serial rerun after the race stays successful and applies nothing new.
	exit, out, err := e.migrateDSN(t.Context(), rawDSN)
	if err != nil || exit != 0 {
		t.Fatalf("idempotent rerun after the race exited %d: %s", exit, out)
	}
	var appliedAfter int64
	if err := conn.QueryRow(t.Context(), "SELECT count(*) FROM "+historyTable).Scan(&appliedAfter); err != nil {
		t.Fatalf("re-read migration history: %s", integration.RedactDSN(err.Error()))
	}
	if appliedAfter != expected {
		t.Fatalf("idempotent rerun changed history from %d to %d rows", applied, appliedAfter)
	}
}
