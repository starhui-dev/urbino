package identity_test

// P03-T02 — tenant scope filtering for list, single read and stats. No read
// may cross the tenant boundary, and cross-tenant misses must be
// indistinguishable from absent records so existence is not leaked.
//
// The seam requires an explicitly scoped store: an unscoped read is a
// compile-time impossibility. Binding the real PostgreSQL store is the main
// agent's integration step; until it links, these tests skip with that exact
// reason (SKIP is not a pass).

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/urbino/tests/identity"
)

// seedTenantWithKeys creates one synthetic tenant/project holding n keys and
// returns the scope plus their public ids.
func seedTenantWithKeys(t *testing.T, store identity.ScopedStore, n int) (identity.Scope, []string) {
	t.Helper()
	scope, err := store.SeedScope(context.Background())
	if err != nil {
		t.Fatalf("SeedScope: %v", err)
	}
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		publicID, err := store.SeedKey(context.Background(), scope, identity.KeySeed{
			UserID:    newID(t),
			Scopes:    []string{"models:invoke"},
			Models:    []string{"model-alpha"},
			ExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("SeedKey: %v", err)
		}
		ids = append(ids, publicID)
	}
	return scope, ids
}

func TestP03T02CrossTenantSingleReadNotFound(t *testing.T) {
	store := openScopedStore(t)
	scopeA, _ := seedTenantWithKeys(t, store, 1)
	_, idsB := seedTenantWithKeys(t, store, 1)

	rec, err := store.GetKeyRecord(context.Background(), scopeA, idsB[0])
	if err == nil {
		t.Fatalf("cross-tenant single read returned a record: %+v", rec)
	}
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("cross-tenant read must be not-found (existence must not leak as forbidden), got: %v", err)
	}
}

func TestP03T02ListFilteredByTenant(t *testing.T) {
	store := openScopedStore(t)
	scopeA, idsA := seedTenantWithKeys(t, store, 2)
	scopeB, idsB := seedTenantWithKeys(t, store, 1)

	listA, err := store.ListKeyRecords(context.Background(), scopeA, identity.ListFilter{Limit: 100})
	if err != nil {
		t.Fatalf("ListKeyRecords(A): %v", err)
	}
	if len(listA) != len(idsA) {
		t.Fatalf("tenant A list length = %d, want %d", len(listA), len(idsA))
	}
	seen := map[string]bool{}
	for _, rec := range listA {
		seen[rec.PublicID] = true
	}
	for _, id := range append(append([]string(nil), idsA...), idsB...) {
		want := seen[id]
		inA := false
		for _, a := range idsA {
			if a == id {
				inA = true
			}
		}
		if want != inA {
			t.Fatalf("tenant A list membership wrong for %q (present=%v, belongsToA=%v)", id, want, inA)
		}
	}

	listB, err := store.ListKeyRecords(context.Background(), scopeB, identity.ListFilter{Limit: 100})
	if err != nil {
		t.Fatalf("ListKeyRecords(B): %v", err)
	}
	if len(listB) != 1 || listB[0].PublicID != idsB[0] {
		t.Fatalf("tenant B list = %+v, want exactly %q", listB, idsB[0])
	}
}

func TestP03T02StatsScopedByTenant(t *testing.T) {
	store := openScopedStore(t)
	scopeA, _ := seedTenantWithKeys(t, store, 2)
	scopeB, _ := seedTenantWithKeys(t, store, 1)

	statsA, err := store.KeyStats(context.Background(), scopeA)
	if err != nil {
		t.Fatalf("KeyStats(A): %v", err)
	}
	statsB, err := store.KeyStats(context.Background(), scopeB)
	if err != nil {
		t.Fatalf("KeyStats(B): %v", err)
	}
	if statsA.TotalKeys != 2 {
		t.Fatalf("tenant A total keys = %d, want 2 (tenant B must not be counted)", statsA.TotalKeys)
	}
	if statsB.TotalKeys != 1 {
		t.Fatalf("tenant B total keys = %d, want 1", statsB.TotalKeys)
	}
}

// TestP03T02MissingAndForeignLookupsIndistinguishable: a record that does not
// exist in the scope and a record that exists in another boundary must both
// surface as the same ErrNotFound, so responses cannot enumerate other
// tenants.
func TestP03T02MissingAndForeignLookupsIndistinguishable(t *testing.T) {
	store := openScopedStore(t)
	scopeA, _ := seedTenantWithKeys(t, store, 1)
	_, idsB := seedTenantWithKeys(t, store, 1)

	_, errForeign := store.GetKeyRecord(context.Background(), scopeA, idsB[0])
	_, errMissing := store.GetKeyRecord(context.Background(), scopeA, "gw_live_nonexistent000000000000000000000000.00000000000000000000000000000000")
	if errForeign == nil || errMissing == nil {
		t.Fatalf("lookups must fail: foreign=%v missing=%v", errForeign, errMissing)
	}
	if errors.Is(errForeign, identity.ErrNotFound) != errors.Is(errMissing, identity.ErrNotFound) {
		t.Fatalf("existence leak: foreign=%v missing=%v", errForeign, errMissing)
	}
}
