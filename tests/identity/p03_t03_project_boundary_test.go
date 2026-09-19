package identity_test

// P03-T03 — project membership boundary. A user without membership in a
// project cannot reference that project's resources, even inside their own
// tenant; tenant isolation keeps holding underneath.

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/urbino/internal/domain"
	"example.com/urbino/tests/identity"
)

// seedProjectWithKey creates one synthetic project with its member user and
// one key, returning the scope, the user and the key's public id.
func seedProjectWithKey(t *testing.T, store identity.ScopedStore, withKey bool) (identity.Scope, domain.PrincipalID, string) {
	t.Helper()
	scope, err := store.SeedScope(context.Background())
	if err != nil {
		t.Fatalf("SeedScope: %v", err)
	}
	user := newID(t)
	if err := store.AddMember(context.Background(), scope, user); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	var publicID string
	if withKey {
		publicID, err = store.SeedKey(context.Background(), scope, identity.KeySeed{
			UserID:    user,
			Scopes:    []string{"models:invoke"},
			Models:    []string{"model-alpha"},
			ExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("SeedKey: %v", err)
		}
	}
	return scope, user, publicID
}

// TestP03T03MembershipRequiredForProjectResources: a non-member's project
// resource access is refused while the member keeps access.
func TestP03T03MembershipRequiredForProjectResources(t *testing.T) {
	store := openScopedStore(t)
	scopeP1, memberP1, _ := seedProjectWithKey(t, store, false)
	scopeP2, _, _ := seedProjectWithKey(t, store, false) // memberP1 is NOT a member of P2

	if err := store.RequireMembership(context.Background(), scopeP1, memberP1); err != nil {
		t.Fatalf("member refused in own project: %v", err)
	}
	err := store.RequireMembership(context.Background(), scopeP2, memberP1)
	if err == nil {
		t.Fatal("non-member accepted for project P2")
	}
	if !errors.Is(err, identity.ErrForbidden) && !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("membership denial must be forbidden or not-found, got: %v", err)
	}
}

// TestP03T03CrossProjectReferenceRejected: a key from project P1 is not
// readable through project P2's scope, same tenant included.
func TestP03T03CrossProjectReferenceRejected(t *testing.T) {
	store := openScopedStore(t)
	_, _, keyP1 := seedProjectWithKey(t, store, true)
	scopeP2, _, _ := seedProjectWithKey(t, store, false)

	rec, err := store.GetKeyRecord(context.Background(), scopeP2, keyP1)
	if err == nil {
		t.Fatalf("cross-project read returned a record: %+v", rec)
	}
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("cross-project reference must be not-found without existence leak, got: %v", err)
	}

	listP2, err := store.ListKeyRecords(context.Background(), scopeP2, identity.ListFilter{Limit: 100})
	if err != nil {
		t.Fatalf("ListKeyRecords(P2): %v", err)
	}
	for _, rec := range listP2 {
		if rec.PublicID == keyP1 {
			t.Fatal("P1 key leaked into P2 list")
		}
	}
}

// TestP03T03TenantBoundaryHoldsUnderProjectChecks: the project boundary is
// additive — cross-tenant reads stay not-found even when project scopes
// coincide by construction.
func TestP03T03TenantBoundaryHoldsUnderProjectChecks(t *testing.T) {
	store := openScopedStore(t)
	scopeT1, _, keyT1 := seedProjectWithKey(t, store, true)
	scopeT2, _, _ := seedProjectWithKey(t, store, false)

	if _, err := store.GetKeyRecord(context.Background(), scopeT2, keyT1); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("cross-tenant read must be not-found, got: %v", err)
	}

	// The P1 key itself remains readable inside its own scope.
	rec, err := store.GetKeyRecord(context.Background(), scopeT1, keyT1)
	if err != nil {
		t.Fatalf("own-scope read failed: %v", err)
	}
	if rec.PublicID != keyT1 {
		t.Fatalf("own-scope read returned %q, want %q", rec.PublicID, keyT1)
	}
}
