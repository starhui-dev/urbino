package identity_test

// P03-T06 — concurrent double bootstrap: exactly one bootstrap wins, losers
// are refused, and neither loser nor winner may reset or invalidate the first
// secret. The exclusive secret-file output (O_EXCL, no overwrite) is the
// CLI's concern; its seam is asserted here and bound by the main agent.

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"example.com/urbino/tests/identity"
)

// TestP03T06ConcurrentBootstrapSingleWinner: N concurrent bootstraps produce
// exactly one success; every loser reports ErrAlreadyBootstrapped; the winner
// authenticates on the admin path.
func TestP03T06ConcurrentBootstrapSingleWinner(t *testing.T) {
	h := newHarness(t)
	adminID := newID(t)

	const n = 8
	type result struct {
		cred identity.AdminCredential
		err  error
	}
	results := make([]result, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			cred, err := h.sys.BootstrapAdmin(context.Background(), identity.BootstrapRequest{
				AdminID: adminID,
				Scopes:  []string{"admin:keys:write"},
			})
			results[i] = result{cred: cred, err: err}
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	var winnerToken string
	for i, r := range results {
		switch {
		case r.err == nil:
			winners++
			winnerToken = r.cred.Token
			if r.cred.Token == "" {
				t.Fatalf("winner %d returned empty token", i)
			}
		case errors.Is(r.err, identity.ErrAlreadyBootstrapped):
			if r.cred.Token != "" {
				t.Fatalf("loser %d leaked credential material", i)
			}
		default:
			t.Fatalf("loser %d returned unexpected error %v (want ErrAlreadyBootstrapped)", i, r.err)
		}
	}
	if winners != 1 {
		t.Fatalf("bootstrap winners = %d, want exactly 1", winners)
	}

	p, err := h.sys.AuthenticateAdmin(context.Background(), bearerReq("GET", "/admin/v1/tenants", withBearer(nil, winnerToken), nil, "203.0.113.10:4444"))
	if err != nil {
		t.Fatalf("winner token rejected on admin path: %v", err)
	}
	if p.AdminID.IsZero() {
		t.Fatal("winner authenticated with a zero admin principal id")
	}
}

// TestP03T06BootstrapDoesNotResetOldSecret: a refused second bootstrap must
// not invalidate, rotate or reset the first admin secret — the original
// token keeps authenticating afterwards.
func TestP03T06BootstrapDoesNotResetOldSecret(t *testing.T) {
	h := newHarness(t)
	cred, err := h.sys.BootstrapAdmin(context.Background(), identity.BootstrapRequest{
		AdminID: newID(t),
		Scopes:  []string{"admin:keys:read"},
	})
	if err != nil {
		t.Fatalf("first BootstrapAdmin: %v", err)
	}

	second, err := h.sys.BootstrapAdmin(context.Background(), identity.BootstrapRequest{
		AdminID: newID(t),
		Scopes:  []string{"admin:keys:read"},
	})
	if !errors.Is(err, identity.ErrAlreadyBootstrapped) {
		t.Fatalf("second bootstrap error = %v, want ErrAlreadyBootstrapped", err)
	}
	if second.Token != "" {
		t.Fatal("refused bootstrap leaked credential material")
	}

	// The first token still authenticates after the refused re-run.
	if _, err := h.sys.AuthenticateAdmin(context.Background(), bearerReq("GET", "/admin/v1/tenants", withBearer(nil, cred.Token), nil, "203.0.113.10:4444")); err != nil {
		t.Fatalf("refused bootstrap disturbed the original secret: %v", err)
	}
}

// TestP03T06SecretOutputIsExclusive: the bootstrap secret file uses O_EXCL —
// the first write succeeds with private permissions, a second write to the
// same path is refused with ErrSecretReuse, and the original content survives
// untouched.
func TestP03T06SecretOutputIsExclusive(t *testing.T) {
	h := newHarness(t)
	cred, err := h.sys.BootstrapAdmin(context.Background(), identity.BootstrapRequest{
		AdminID: newID(t),
		Scopes:  []string{"admin:keys:read"},
	})
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	writer := h.sys.SecretWriter()
	path := t.TempDir() + "/bootstrap.secret"

	if err := writer.WriteNew(path, cred.Token); err != nil {
		t.Fatalf("first secret write failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("secret file missing: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secret file permissions = %v, want 0600", info.Mode().Perm())
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read secret file: %v", err)
	}
	if string(first) != cred.Token+"\n" {
		t.Fatalf("secret file content mismatch (len %d)", len(first))
	}

	if err := writer.WriteNew(path, "different-value"); !errors.Is(err, identity.ErrSecretReuse) {
		t.Fatalf("second write error = %v, want ErrSecretReuse", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read secret file after refused write: %v", err)
	}
	if string(second) != cred.Token+"\n" {
		t.Fatal("refused write altered the original secret output")
	}
}
