package identity_test

// P03-T09 — one key may carry a policy of several allowed models: allowed
// models authorize, anything outside the policy is denied, an empty policy
// denies everything (fail closed), and two keys may share the same policy.

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/urbino/internal/domain"
	"example.com/urbino/tests/identity"
)

// TestP03T09OneKeyMultipleAllowedModels: a single key with a two-model policy
// authorizes both listed models and denies a third.
func TestP03T09OneKeyMultipleAllowedModels(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h,
		[]string{"models:read"},
		[]string{"model-alpha", "model-beta"},
		h.clk.Now().Add(time.Hour),
	)
	p := authPublic(t, h, bearerReq("POST", "/v1/chat/completions", withBearer(nil, issued.Secret), nil, "198.51.100.7:5555"))

	for _, model := range []string{"model-alpha", "model-beta"} {
		if err := h.sys.AuthorizeModel(p, model); err != nil {
			t.Fatalf("allowed model %q refused: %v", model, err)
		}
	}
	if err := h.sys.AuthorizeModel(p, "model-gamma"); err == nil {
		t.Fatal("model outside the policy accepted")
	}
}

// TestP03T09EmptyPolicyRejectedAtCreation: a key without a model policy is
// refused at creation, so a wildcard or empty-policy key can never exist —
// the fail-closed default is enforced at the boundary, not per request.
func TestP03T09EmptyPolicyRejectedAtCreation(t *testing.T) {
	h := newHarness(t)
	_, err := h.sys.IssueKey(context.Background(), identity.IssueKeyRequest{
		Scope:     identity.Scope{TenantID: domain.TenantID(newID(t)), ProjectID: domain.ProjectID(newID(t))},
		UserID:    newID(t),
		Scopes:    []string{"models:invoke"},
		Models:    nil,
		ExpiresAt: h.clk.Now().Add(time.Hour),
	})
	if err == nil {
		t.Fatal("key without model policy accepted at creation")
	}
	if !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("expected rejection, got: %v", err)
	}
}

// TestP03T09PolicyIsPerKeyAndShareable: two keys on the same user may share
// one allowed-model policy with identical verdicts, while a differently
// policed key keeps its own verdicts — sharing a policy never widens it.
func TestP03T09PolicyIsPerKeyAndShareable(t *testing.T) {
	h := newHarness(t)
	shared := []string{"model-alpha", "model-beta"}
	issued1, tenant, project := issueKey(t, h, []string{"models:read"}, shared, h.clk.Now().Add(time.Hour))

	// Second key, same user/scope, same shared policy.
	userID := issued1.Record.UserID
	issued2, err := h.sys.IssueKey(context.Background(), identity.IssueKeyRequest{
		Scope:     identity.Scope{TenantID: tenant, ProjectID: project},
		UserID:    userID,
		Scopes:    []string{"models:read"},
		Models:    shared,
		ExpiresAt: h.clk.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("IssueKey(second): %v", err)
	}
	// Third key, same user, narrower policy.
	issued3, err := h.sys.IssueKey(context.Background(), identity.IssueKeyRequest{
		Scope:     identity.Scope{TenantID: tenant, ProjectID: project},
		UserID:    userID,
		Scopes:    []string{"models:read"},
		Models:    []string{"model-beta"},
		ExpiresAt: h.clk.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("IssueKey(third): %v", err)
	}

	p1 := authPublic(t, h, bearerReq("POST", "/v1/chat/completions", withBearer(nil, issued1.Secret), nil, "198.51.100.7:5561"))
	p2 := authPublic(t, h, bearerReq("POST", "/v1/chat/completions", withBearer(nil, issued2.Secret), nil, "198.51.100.7:5562"))
	p3 := authPublic(t, h, bearerReq("POST", "/v1/chat/completions", withBearer(nil, issued3.Secret), nil, "198.51.100.7:5563"))

	for _, model := range shared {
		if err := h.sys.AuthorizeModel(p1, model); err != nil {
			t.Fatalf("key1 denied shared policy model %q: %v", model, err)
		}
		if err := h.sys.AuthorizeModel(p2, model); err != nil {
			t.Fatalf("key2 with identical policy denied %q: %v", model, err)
		}
	}
	if err := h.sys.AuthorizeModel(p1, "model-gamma"); err == nil {
		t.Fatal("key1 authorized model outside shared policy")
	}
	if err := h.sys.AuthorizeModel(p2, "model-gamma"); err == nil {
		t.Fatal("key2 authorized model outside shared policy")
	}
	// The narrower key may not be widened by its neighbour's policy.
	if err := h.sys.AuthorizeModel(p3, "model-alpha"); err == nil {
		t.Fatal("narrower key widened by shared policy of sibling key")
	}
	if err := h.sys.AuthorizeModel(p3, "model-beta"); err != nil {
		t.Fatalf("narrower key denied its own model: %v", err)
	}
}
