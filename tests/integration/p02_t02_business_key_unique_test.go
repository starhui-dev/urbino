package integration_test

import (
	"testing"

	"example.com/urbino/tests/integration"
)

// TestP02T02BusinessKeysAreUnique proves the phase 02 uniqueness contracts at
// the database level: the request idempotency key is unique per tenant, the
// (tenant_id, request_id, attempt_no) triple is unique, and the settlement
// business key is unique per tenant. Each subtest first asserts the unique
// index exists over exactly the contracted columns, then plants real
// duplicate inserts.
func TestP02T02BusinessKeysAreUnique(t *testing.T) {
	t.Run("requests_idempotency_digest_is_unique_per_tenant", func(t *testing.T) {
		f := newFixture(t)
		f.assertUniqueCovers("requests", "tenant_id", "idempotency_digest")
		tenantA := f.seed("tenants", nil)
		tenantB := f.seed("tenants", nil)
		digest := []byte("synthetic-idempotency-digest-001")

		f.seed("requests", map[string]any{"tenant_id": tenantA["id"], "idempotency_digest": digest})

		_, err := f.trySeed("requests", map[string]any{"tenant_id": tenantA["id"], "idempotency_digest": digest}, 0)
		if code := pgErrCode(t, err); code != "23505" {
			t.Fatalf("expected unique_violation 23505 for duplicate idempotency digest in one tenant, got %s: %s", code, integration.RedactDSN(err.Error()))
		}

		// The same digest under another tenant must stay insertable: the key
		// is tenant-scoped, not global.
		f.seed("requests", map[string]any{"tenant_id": tenantB["id"], "idempotency_digest": digest})
	})

	t.Run("request_attempts_unique_per_request_and_attempt_no", func(t *testing.T) {
		f := newFixture(t)
		f.assertUniqueCovers("request_attempts", "tenant_id", "request_id", "attempt_no")
		request := f.seed("requests", nil)

		first := map[string]any{
			"tenant_id":  request["tenant_id"],
			"request_id": request["id"],
			"attempt_no": int32(1),
		}
		f.seed("request_attempts", first)

		_, err := f.trySeed("request_attempts", map[string]any{
			"tenant_id":  request["tenant_id"],
			"request_id": request["id"],
			"attempt_no": int32(1),
		}, 0)
		if code := pgErrCode(t, err); code != "23505" {
			t.Fatalf("expected unique_violation 23505 for duplicate (tenant, request, attempt_no), got %s: %s", code, integration.RedactDSN(err.Error()))
		}

		// The next attempt number for the same request is normal.
		f.seed("request_attempts", map[string]any{
			"tenant_id":  request["tenant_id"],
			"request_id": request["id"],
			"attempt_no": int32(2),
		})
	})

	t.Run("settlements_business_key_is_unique_per_tenant", func(t *testing.T) {
		f := newFixture(t)
		f.assertUniqueCovers("settlements", "tenant_id", "business_key")
		requestA := f.seed("requests", nil)
		requestB := f.seed("requests", nil)
		businessKey := "synthetic-settlement-key-001"

		f.seed("settlements", map[string]any{
			"tenant_id":    requestA["tenant_id"],
			"request_id":   requestA["id"],
			"business_key": businessKey,
		})

		_, err := f.trySeed("settlements", map[string]any{
			"tenant_id":    requestA["tenant_id"],
			"request_id":   requestA["id"],
			"business_key": businessKey,
		}, 0)
		if code := pgErrCode(t, err); code != "23505" {
			t.Fatalf("expected unique_violation 23505 for duplicate settlement business key in one tenant, got %s: %s", code, integration.RedactDSN(err.Error()))
		}

		// Another tenant may reuse the same business key.
		f.seed("settlements", map[string]any{
			"tenant_id":    requestB["tenant_id"],
			"request_id":   requestB["id"],
			"business_key": businessKey,
		})
	})
}
