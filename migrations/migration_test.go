package migrations

import (
	"os"
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
		"price_version_id uuid NOT NULL REFERENCES urbino.price_versions(id)",
		"FOREIGN KEY (tenant_id,request_id,usage_event_id) REFERENCES urbino.usage_events(tenant_id,request_id,id)",
		"input_total bigint CHECK (input_total IS NULL OR input_total >= 0)",
		"FOREIGN KEY (transaction_id,currency) REFERENCES urbino.journal_transactions(id,currency)",
		"CREATE CONSTRAINT TRIGGER journal_entries_balance",
		"journal rows are immutable",
		"published price rows are immutable",
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

func TestUsageEventQueryIncludesTenantScope(t *testing.T) {
	b, err := os.ReadFile("../sql/queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "-- name: InsertUsageEvent")
	if start < 0 {
		t.Fatal("InsertUsageEvent query missing")
	}
	end := strings.Index(s[start:], "\n-- name:")
	if end < 0 {
		end = len(s) - start
	}
	query := s[start : start+end]
	if !strings.Contains(query, "tenant_id") {
		t.Fatal("InsertUsageEvent must write tenant_id for composite tenant scope")
	}
}
