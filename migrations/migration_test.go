package migrations

import (
	"strings"
	"testing"
)

func TestInitialMigrationContainsTenantAndLedgerGuards(t *testing.T) {
	b, err := FS.ReadFile("001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"FOREIGN KEY (tenant_id,project_id) REFERENCES urbino.projects(tenant_id,id)",
		"UNIQUE (request_id,attempt_no)",
		"business_key text NOT NULL UNIQUE",
		"CREATE CONSTRAINT TRIGGER journal_entries_balance",
		"journal rows are immutable",
		"GRANT SELECT ON ALL TABLES IN SCHEMA urbino TO urbino_runtime",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("migration missing invariant %q", want)
		}
	}
	if strings.Contains(s, "GRANT SELECT,INSERT,UPDATE ON ALL TABLES") {
		t.Fatal("runtime role received broad update grant")
	}
}
