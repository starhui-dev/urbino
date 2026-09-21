// harness_test.go holds the deterministic collaborators for the stage 03
// boundary tests: an injectable clock, a revocation backend with controllable
// failures, a capturing log handler, and request builders. Everything is
// synthetic; no real provider, database or secret is ever contacted.
package identity_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/urbino/internal/domain"
	"example.com/urbino/tests/identity"
)

// baseTime is the fixed wall-clock origin every test starts from.
var baseTime = time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)

// fakeClock is a deterministic clock.Clock advanced explicitly by tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: baseTime} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// flakyBackend is an in-memory RevocationBackend whose failure modes tests
// switch on to prove fail-closed behaviour.
type flakyBackend struct {
	mu            sync.Mutex
	entries       map[string]identity.AuthState
	failGet       bool
	failPut       bool
	suppressWrite bool
	suppressDel   bool
	puts          int
	deletes       int
}

func newFlakyBackend() *flakyBackend {
	return &flakyBackend{entries: make(map[string]identity.AuthState)}
}

func (b *flakyBackend) Get(_ context.Context, publicID string) (identity.AuthState, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failGet {
		return identity.AuthState{}, false, errors.New("backend unavailable")
	}
	st, ok := b.entries[publicID]
	return st, ok, nil
}

func (b *flakyBackend) Put(_ context.Context, publicID string, state identity.AuthState, _ time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.puts++
	if b.failPut {
		return errors.New("backend write failed")
	}
	if !b.suppressWrite {
		b.entries[publicID] = state
	}
	return nil
}

func (b *flakyBackend) Delete(_ context.Context, publicID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deletes++
	if b.suppressDel {
		return nil
	}
	delete(b.entries, publicID)
	return nil
}

func (b *flakyBackend) has(publicID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.entries[publicID]
	return ok
}

// get returns a snapshot of the shared state for assertions.
func (b *flakyBackend) get(publicID string) (identity.AuthState, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	st, ok := b.entries[publicID]
	return st, ok
}

// captureHandler is a slog handler recording every formatted entry so tests
// can prove secret material never reaches logs.
type captureHandler struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	entries []string
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	line := fmt.Sprintf("%s %s %s", r.Time.Format(time.RFC3339Nano), r.Level, r.Message)
	r.Attrs(func(a slog.Attr) bool {
		line += fmt.Sprintf(" %s=%v", a.Key, a.Value)
		return true
	})
	h.entries = append(h.entries, line)
	h.buf.WriteString(line + "\n")
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) dump() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.buf.String()
}

// harness carries one System instance with its collaborators.
type harness struct {
	sys     identity.System
	clk     *fakeClock
	backend *flakyBackend
	logs    *captureHandler
}

// newHarness builds a linked System with deterministic collaborators, or
// skips with the documented reason when the implementation is not linked.
// The skip is recorded as SKIP/BLOCKED in evidence — never as a pass.
func newHarness(t *testing.T) *harness {
	t.Helper()
	clk := newFakeClock()
	backend := newFlakyBackend()
	logs := &captureHandler{}
	sys, err := identity.New(context.Background(), identity.Options{
		Clock:          clk,
		Peppers:        testPeppers(t),
		ActivePepperID: "pepper-v1",
		Backend:        backend,
		Logger:         slog.New(logs),
	})
	if errors.Is(err, identity.ErrUnbound) {
		t.Skipf("internal/auth not linked in default build; run `go test -tags identityimpl ./tests/identity -count=1 -v` after the implementation lands (SKIP is not a pass; see evidence/tasks/03/identity-boundary-tests.md)")
	}
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	return &harness{sys: sys, clk: clk, backend: backend, logs: logs}
}

// testPeppers returns two independent synthetic peppers, one per digest key
// id, so pepper separation is observable. They live only inside the test
// process and are never written to evidence.
func testPeppers(t *testing.T) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte, 2)
	for _, id := range []string{"pepper-v1", "pepper-v2"} {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			t.Fatalf("synthetic pepper: %v", err)
		}
		out[id] = b
	}
	return out
}

// openScopedStore binds the PostgreSQL-backed ScopedStore or skips with the
// reason recorded in the P03-T02/T03 evidence mapping.
func openScopedStore(t *testing.T) identity.ScopedStore {
	t.Helper()
	dsn := os.Getenv("URBINO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("PostgreSQL scoped store requires explicit URBINO_TEST_POSTGRES_DSN (SKIP is not a pass)")
	}
	store, err := identity.NewScopedStore(context.Background(), dsn)
	switch {
	case errors.Is(err, identity.ErrUnbound):
		t.Skip("ScopedStore not linked in default build (SKIP is not a pass)")
	case errors.Is(err, identity.ErrPGAdapterPending):
		t.Skip("ScopedStore requires the main agent's PostgreSQL adapter; internal/auth is linked but storage binding is pending (SKIP is not a pass)")
	case err != nil:
		t.Skipf("PostgreSQL scoped store unavailable: %v (SKIP is not a pass; main agent integration required)", err)
	}
	if closer, ok := store.(interface{ Close() }); ok {
		t.Cleanup(closer.Close)
	}
	return store
}

// newID returns a fresh synthetic domain UUID.
func newID(t *testing.T) domain.PrincipalID {
	t.Helper()
	id, err := domain.NewUUID()
	if err != nil {
		t.Fatalf("synthetic uuid: %v", err)
	}
	return domain.PrincipalID(id)
}

// syntheticSecret mints a throwaway bearer-shaped string for negative header
// tests. It is never valid key material and never written to evidence.
func syntheticSecret(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("synthetic secret: %v", err)
	}
	return identity.KeyPrefix + "synth" + hex.EncodeToString(b)[:16] + "." + hex.EncodeToString(b)
}

// issueKey is the common creation helper: fresh scope/user ids and the
// requested attributes, returning the one-time secret plus record.
func issueKey(t *testing.T, h *harness, scopes, models []string, expires time.Time) (identity.IssuedKey, domain.TenantID, domain.ProjectID) {
	t.Helper()
	issued, err := h.sys.IssueKey(context.Background(), identity.IssueKeyRequest{
		Scope:     identity.Scope{TenantID: domain.TenantID(newID(t)), ProjectID: domain.ProjectID(newID(t))},
		UserID:    newID(t),
		Scopes:    scopes,
		Models:    models,
		ExpiresAt: expires,
	})
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}
	if err := identity.ValidateIssuedSecret(issued.Secret); err != nil {
		t.Fatalf("issued key violates shape contract: %v", err)
	}
	return issued, issued.Record.TenantID, issued.Record.ProjectID
}

// bearerReq builds a Request carrying exactly the supplied header pairs.
func bearerReq(method, path string, header http.Header, query url.Values, remote string) *identity.Request {
	return &identity.Request{
		Method:     method,
		Path:       path,
		Header:     header,
		Query:      query,
		RemoteAddr: remote,
	}
}

// withBearer sets one Authorization header value.
func withBearer(h http.Header, value string) http.Header {
	out := http.Header{}
	for k, vs := range h {
		out[k] = append([]string(nil), vs...)
	}
	out.Set("Authorization", "Bearer "+value)
	return out
}

// authPublic authenticates and requires success.
func authPublic(t *testing.T, h *harness, req *identity.Request) identity.Principal {
	t.Helper()
	p, err := h.sys.AuthenticatePublic(context.Background(), req)
	if err != nil {
		t.Fatalf("AuthenticatePublic: %v", err)
	}
	return p
}

// errText renders an error for secret-leak scanning, including the cause
// chain the way callers and logs would see it.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return strings.Join([]string{err.Error(), fmt.Sprintf("%v", err)}, " | ")
}
