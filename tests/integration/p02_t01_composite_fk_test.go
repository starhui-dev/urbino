package integration_test

import (
	"testing"

	"example.com/urbino/tests/integration"
)

// TestP02T01CrossTenantCompositeForeignKeyRejected proves that cross-tenant
// resource references are rejected by the database itself, not merely by
// handler code: requests carries ONE composite foreign key to projects over
// exactly (tenant_id, project_id), so a row in tenant B cannot reference a
// project of tenant A even with a colliding id.
func TestP02T01CrossTenantCompositeForeignKeyRejected(t *testing.T) {
	f := newFixture(t)

	fks, err := f.foreignKeys("requests")
	if err != nil {
		t.Fatalf("introspect foreign keys on requests: %s", integration.RedactDSN(err.Error()))
	}
	var projectFK *fkConstraint
	for i := range fks {
		if fks[i].parent == "projects" {
			projectFK = &fks[i]
			break
		}
	}
	if projectFK == nil {
		t.Fatalf("requests has no foreign key to projects; merged schema violates the composite-key contract")
	}
	if len(projectFK.pairs) != 2 {
		t.Fatalf("requests->projects foreign key %s must be composite (tenant_id, project_id), got %d column pairs", projectFK.name, len(projectFK.pairs))
	}
	seen := map[string]string{}
	for _, p := range projectFK.pairs {
		seen[p.child] = p.parent
	}
	if seen["tenant_id"] != "tenant_id" || seen["project_id"] != "id" {
		t.Fatalf("requests->projects foreign key %s must map (tenant_id->tenant_id, project_id->id), got %v", projectFK.name, projectFK.pairs)
	}

	tenantA := f.seed("tenants", nil)
	tenantB := f.seed("tenants", nil)
	projectA := f.seed("projects", map[string]any{"tenant_id": tenantA["id"]})

	// Positive control: the same-tenant composite reference is accepted, so
	// the rejection below is caused by the cross-tenant reference itself.
	f.seed("requests", map[string]any{"tenant_id": tenantA["id"], "project_id": projectA["id"]})

	// The violation under test: tenant B references tenant A's project id.
	_, err = f.trySeed("requests", map[string]any{
		"tenant_id":  tenantB["id"],
		"project_id": projectA["id"],
	}, 0)
	if code := pgErrCode(t, err); code != "23503" {
		t.Fatalf("expected foreign_key_violation 23503 for cross-tenant project reference, got %s: %s", code, integration.RedactDSN(err.Error()))
	}
}
