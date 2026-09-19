// Package identity defines the stage 03 authentication boundary seam used by
// the tests/identity suite (P03-T01..T09 of checklists/test-matrix.csv).
//
// The seam states the observable contract that internal/auth must satisfy,
// derived from progress/plans/03.md "公共契约与不变量" and docs/06-auth-admin.md:
//
//   - Public key shape gw_live_<public_id>.<secret>; secret >= 32 crypto/rand
//     bytes; persistence keeps only public_id, pepper id, HMAC-SHA-256 digest
//     and metadata — never the plaintext secret (one-time disclosure).
//   - Verification: lookup by public_id, HMAC-SHA-256 under an independent
//     pepper, constant-time comparison.
//   - Public and admin authentication are independent paths, prefixes and
//     scopes; a public key never authenticates on the admin path and vice
//     versa. Admin roles are never derivable from public principals.
//   - Only canonical auth headers are accepted (Authorization Bearer,
//     x-api-key, x-goog-api-key, all carrying the gateway key). Conflicting
//     or duplicated credentials are rejected without picking one; any key in
//     the URL query is rejected.
//   - Revocation cache TTL is at most 5 seconds; cache miss, backend error or
//     unknown auth_version must fail closed (reject), never serve stale allow.
//   - Every resource read is explicitly tenant/project scoped; cross-boundary
//     access is reported as not-found without leaking existence.
//
// Binding: the default build is not linked to internal/auth. New and
// NewScopedStore return ErrUnbound and every test skips with that reason —
// a skip is NOT a pass. The linked adapter lives in binding_identityimpl.go
// behind the `identityimpl` build tag:
//
//	go test -tags identityimpl ./tests/identity -count=1 -v
//
// If internal/auth's exported surface differs from the adapter's references,
// fix binding_identityimpl.go only; the assertions in the *_test.go files are
// the approved contract and must not be weakened to fit an implementation.
//
// The suite uses synthetic identifiers and secrets only, never logs or
// evidences full secret material, and is safe for `go test -count=2`.
package identity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"example.com/urbino/internal/clock"
	"example.com/urbino/internal/domain"
)

// Contract constants from progress/plans/03.md.
const (
	// KeyPrefix is the mandatory prefix of every public key.
	KeyPrefix = "gw_live_"
	// SecretSeparator separates public_id from the secret portion.
	SecretSeparator = "."
	// MinSecretBytes is the minimum entropy carried by the secret portion.
	MinSecretBytes = 32
	// MaxRevocationTTL is the upper bound for the revocation cache window.
	MaxRevocationTTL = 5 * time.Second
)

// Sentinel errors. The adapter MUST map internal/auth failures onto these so
// tests assert causes, not error strings. Unknown mappings fail tests by
// design: an unmapped error means the boundary contract is not observable.
var (
	// ErrUnbound reports that no implementation is linked (default build).
	ErrUnbound = errors.New("identity: implementation not linked (build with -tags identityimpl)")
	// ErrPGAdapterPending reports that the implementation is linked but the
	// PostgreSQL-backed ScopedStore adapter is still the main agent's
	// integration responsibility.
	ErrPGAdapterPending = errors.New("identity: scoped store requires the main agent's PostgreSQL adapter")

	// ErrUnauthorized is the generic rejection class.
	ErrUnauthorized = errors.New("identity: unauthorized")
	// ErrExpired is a rejection caused by expires_at in the past.
	ErrExpired = fmt.Errorf("identity: key expired: %w", ErrUnauthorized)
	// ErrRevoked is a rejection caused by revocation.
	ErrRevoked = fmt.Errorf("identity: key revoked: %w", ErrUnauthorized)
	// ErrStaleVersion is a rejection caused by an auth_version mismatch
	// between the cache and the authoritative record.
	ErrStaleVersion = fmt.Errorf("identity: stale auth version: %w", ErrUnauthorized)
	// ErrAmbiguousCredentials rejects conflicting or duplicated auth headers.
	ErrAmbiguousCredentials = fmt.Errorf("identity: conflicting authentication credentials: %w", ErrUnauthorized)
	// ErrQueryKey rejects any key material carried in the URL query.
	ErrQueryKey = fmt.Errorf("identity: key in URL query rejected: %w", ErrUnauthorized)

	// ErrForbidden is the authenticated-but-not-allowed class.
	ErrForbidden = errors.New("identity: forbidden")
	// ErrScopeDenied rejects an operation outside the subject's scopes.
	ErrScopeDenied = fmt.Errorf("identity: scope denied: %w", ErrForbidden)
	// ErrModelDenied rejects a model outside the key's model policy.
	ErrModelDenied = fmt.Errorf("identity: model not allowed by policy: %w", ErrForbidden)

	// ErrNotFound reports an absent or out-of-scope resource. Cross-tenant
	// and cross-project access MUST map here, not to ErrForbidden, so that
	// existence in another boundary is not leaked.
	ErrNotFound = errors.New("identity: resource not found")

	// ErrCacheUnavailable reports a revocation cache backend failure. The
	// authentication result must still be a rejection (fail closed).
	ErrCacheUnavailable = errors.New("identity: revocation cache unavailable")

	// ErrAlreadyBootstrapped rejects a second bootstrap of the same
	// installation; the first secret must stay valid.
	ErrAlreadyBootstrapped = errors.New("identity: admin already bootstrapped")
	// ErrSecretReuse refuses to overwrite an existing secret output file.
	ErrSecretReuse = errors.New("identity: secret output already exists")
)

// Scope identifies the tenant/project boundary of a resource or request.
// TenantID is always required; ProjectID is required for project-scoped
// operations and may be zero for tenant-wide administrative reads.
type Scope struct {
	TenantID  domain.TenantID
	ProjectID domain.ProjectID
}

// KeyRecord is what persistence is allowed to keep about a public key. It has
// no field for the plaintext secret by construction.
type KeyRecord struct {
	PublicID     string
	DigestKeyID  string // pepper version used for the digest
	Digest       []byte // HMAC-SHA-256 of the secret under that pepper
	TenantID     domain.TenantID
	ProjectID    domain.ProjectID
	UserID       domain.PrincipalID
	Scopes       []string
	Models       []string // allowed model policy; empty denies everything
	Status       string   // active / disabled / revoked
	ExpiresAt    time.Time
	RevokedAt    time.Time // zero while active
	AuthVersion  int64
	CreatedAtUTC domain.UTC
}

// IssuedKey is the one-time creation output. Secret is shown exactly once.
type IssuedKey struct {
	Secret string
	Record KeyRecord
}

// IssueKeyRequest creates a public key inside a scope.
type IssueKeyRequest struct {
	Scope     Scope
	UserID    domain.PrincipalID
	Scopes    []string
	Models    []string
	ExpiresAt time.Time
}

// Principal is an authenticated public identity.
type Principal struct {
	TenantID    domain.TenantID
	ProjectID   domain.ProjectID
	UserID      domain.PrincipalID
	KeyID       string // public_id
	Scopes      []string
	Models      []string
	AuthVersion int64
}

// AdminPrincipal is an authenticated administrative identity. It shares no
// fields with Principal beyond ID semantics and cannot be produced from a
// public key.
type AdminPrincipal struct {
	AdminID domain.PrincipalID
	Scopes  []string
}

// AdminCredential is the one-time admin secret output.
type AdminCredential struct {
	Token     string
	Principal AdminPrincipal
}

// BootstrapRequest creates the first admin principal of an installation.
type BootstrapRequest struct {
	AdminID domain.PrincipalID
	Scopes  []string
}

// IssueAdminTokenRequest issues an additional admin token.
type IssueAdminTokenRequest struct {
	AdminID domain.PrincipalID
	Scopes  []string
	TTL     time.Duration
}

// Request is the observable inbound request view available to authentication.
// RemoteAddr is the only trusted client address; forwarded headers must be
// ignored until a trusted proxy boundary exists.
type Request struct {
	Method     string
	Path       string
	Header     http.Header
	Query      url.Values
	RemoteAddr string
}

// AuthState is a cached authorization snapshot for one public_id.
type AuthState struct {
	Record      KeyRecord
	AuthVersion int64
}

// RevocationBackend is the cache backing store. Production binds Valkey;
// tests bind an in-memory fake with controllable failures. Errors must be
// propagated, never swallowed into a cache miss with stale data.
type RevocationBackend interface {
	Get(ctx context.Context, publicID string) (AuthState, bool, error)
	Put(ctx context.Context, publicID string, state AuthState, ttl time.Duration) error
	Delete(ctx context.Context, publicID string) error
}

// Options wires deterministic collaborators into the implementation. Clock is
// required so revocation windows are testable without sleeping. Peppers maps
// digest key ids to independent pepper secrets; ActivePepperID selects the
// id used for new digests. RevocationTTL is the cache window requested from
// the implementation; it must stay within MaxRevocationTTL.
type Options struct {
	Clock          clock.Clock
	Peppers        map[string][]byte
	ActivePepperID string
	Backend        RevocationBackend
	RevocationTTL  time.Duration
	// Logger receives authentication decisions; tests attach a capturing
	// handler to prove secrets never reach logs (P03-T08).
	Logger *slog.Logger
}

// System is the authentication boundary seam bound to internal/auth.
type System interface {
	// IssueKey creates a public key and returns the plaintext secret once.
	IssueKey(ctx context.Context, req IssueKeyRequest) (IssuedKey, error)
	// RevokeKey revokes a key and must invalidate cached state.
	RevokeKey(ctx context.Context, scope Scope, publicID string, at time.Time) error
	// AuthenticatePublic verifies a request on the public path.
	AuthenticatePublic(ctx context.Context, req *Request) (Principal, error)
	// CheckPublicScope enforces the public key's scope list.
	CheckPublicScope(p Principal, scope string) error
	// AuthorizeModel enforces the key's model policy before dispatch.
	AuthorizeModel(p Principal, model string) error

	// BootstrapAdmin creates the one and only initial admin identity.
	BootstrapAdmin(ctx context.Context, req BootstrapRequest) (AdminCredential, error)
	// IssueAdminToken issues further admin tokens on the admin path.
	IssueAdminToken(ctx context.Context, req IssueAdminTokenRequest) (AdminCredential, error)
	// AuthenticateAdmin verifies a request on the admin path. It must never
	// accept public key material.
	AuthenticateAdmin(ctx context.Context, req *Request) (AdminPrincipal, error)
	// CheckAdminScope enforces admin RBAC scopes.
	CheckAdminScope(p AdminPrincipal, scope string) error

	// RevocationTTL reports the configured cache window; it must be in
	// (0, MaxRevocationTTL].
	RevocationTTL() time.Duration

	// SecretWriter exposes the exclusive secret-file output (O_EXCL, no
	// overwrite) used by bootstrap.
	SecretWriter() SecretFileWriter
}

// SecretFileWriter writes a secret to a new file, refusing to overwrite.
type SecretFileWriter interface {
	WriteNew(path, secret string) error
}

// KeySeed carries test-chosen attributes for store-side seeding. The store
// generates the crypto material through the implementation's own path.
type KeySeed struct {
	UserID    domain.PrincipalID
	Scopes    []string
	Models    []string
	ExpiresAt time.Time
}

// ListFilter constrains scoped list queries.
type ListFilter struct {
	Status string
	Limit  int
}

// Stats is a scope-scoped aggregate.
type Stats struct {
	TotalKeys   int
	ActiveKeys  int
	RevokedKeys int
}

// ScopedStore is the persistence seam for tenant/project boundary tests. All
// reads take an explicit Scope parameter: an unscoped read is a compile-time
// impossibility. Seeding methods exist for tests only and bind to the real
// storage write path.
type ScopedStore interface {
	// SeedScope creates one synthetic tenant and project, returning them.
	SeedScope(ctx context.Context) (Scope, error)
	// SeedKey inserts a key record inside the scope.
	SeedKey(ctx context.Context, scope Scope, seed KeySeed) (publicID string, err error)
	// AddMember grants a user membership in the project.
	AddMember(ctx context.Context, scope Scope, user domain.PrincipalID) error
	// RequireMembership errors when the user lacks project membership.
	RequireMembership(ctx context.Context, scope Scope, user domain.PrincipalID) error
	// GetKeyRecord reads one key inside the scope.
	GetKeyRecord(ctx context.Context, scope Scope, publicID string) (KeyRecord, error)
	// ListKeyRecords lists keys inside the scope only.
	ListKeyRecords(ctx context.Context, scope Scope, filter ListFilter) ([]KeyRecord, error)
	// KeyStats aggregates keys inside the scope only.
	KeyStats(ctx context.Context, scope Scope) (Stats, error)
}

// ValidateIssuedSecret enforces the public key shape contract:
// gw_live_<public_id>.<secret> with a non-empty public_id and at least
// MinSecretBytes of secret material.
func ValidateIssuedSecret(secret string) error {
	if len(secret) <= len(KeyPrefix) || secret[:len(KeyPrefix)] != KeyPrefix {
		return fmt.Errorf("key must start with %q: %w", KeyPrefix, ErrUnauthorized)
	}
	rest := secret[len(KeyPrefix):]
	dot := -1
	for i := 0; i < len(rest); i++ {
		if rest[i] == '.' {
			if dot != -1 {
				return fmt.Errorf("secret must contain exactly one separator: %w", ErrUnauthorized)
			}
			dot = i
		}
	}
	if dot <= 0 {
		return fmt.Errorf("public_id must be non-empty: %w", ErrUnauthorized)
	}
	if dot == len(rest)-1 {
		return fmt.Errorf("secret portion must be non-empty: %w", ErrUnauthorized)
	}
	if n := len(rest) - dot - 1; n < MinSecretBytes {
		return fmt.Errorf("secret portion carries %d bytes, need >= %d: %w", n, MinSecretBytes, ErrUnauthorized)
	}
	return nil
}

// Redact renders secret material evidence-safe. It never returns the input.
func Redact(secret string) string {
	if secret == "" {
		return "<empty>"
	}
	return fmt.Sprintf("<redacted len=%d prefix=%q>", len(secret), KeyPrefix)
}
