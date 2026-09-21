//go:build identityimpl

// Adapter binding the seam (seam.go) to internal/auth's Service.
//
// internal/auth.Service mirrors the seam's shape; this file converts between
// the seam's implementation-agnostic types and auth's types, maps impl
// sentinels onto the seam taxonomy via the IsXxx classifiers, and adapts the
// test RevocationBackend onto auth.RevocationBackend. No assertion logic
// lives here — the *_test.go files are the approved contract.
//
// Build & run:
//
//	go test -tags identityimpl ./tests/identity -count=1 -v
package identity

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/clock"
	"example.com/urbino/internal/domain"
	"example.com/urbino/internal/storage/migrate"
	"example.com/urbino/internal/storage/postgres"
)

type system struct {
	impl *auth.Service
	ttl  time.Duration
}

// New builds the seam System over internal/auth.New.
func New(ctx context.Context, opts Options) (System, error) {
	aopts := auth.Options{
		Clock:          opts.Clock,
		Peppers:        opts.Peppers,
		ActivePepperID: opts.ActivePepperID,
		RevocationTTL:  opts.RevocationTTL,
		Logger:         opts.Logger,
	}
	if aopts.Clock == nil {
		aopts.Clock = clock.Real{}
	}
	if opts.Backend != nil {
		aopts.Backend = backendBridge{opts.Backend}
	}
	impl, ttl, err := auth.New(ctx, aopts)
	if err != nil {
		return nil, mapErr(err)
	}
	return &system{impl: impl, ttl: ttl}, nil
}

// mapErr translates internal/auth sentinels onto the seam taxonomy. Unknown
// errors land in the unauthorized class: a test asserting a specific cause
// fails loudly when the boundary's classification is not observable.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case auth.IsExpired(err):
		return ErrExpired
	case auth.IsRevoked(err):
		return ErrRevoked
	case auth.IsStaleVersion(err):
		return ErrStaleVersion
	case auth.IsAmbiguousCredentials(err):
		return ErrAmbiguousCredentials
	case auth.IsQueryKey(err):
		return ErrQueryKey
	case auth.IsCacheUnavailable(err):
		return fmt.Errorf("%w: %v", ErrCacheUnavailable, err)
	case auth.IsAlreadyBootstrapped(err):
		return ErrAlreadyBootstrapped
	case auth.IsModelDenied(err):
		return ErrModelDenied
	case auth.IsNotFound(err):
		return ErrNotFound
	case auth.IsScopeDenied(err):
		return ErrScopeDenied
	default:
		return fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}
}

func seamRecord(r auth.KeyRecord) KeyRecord {
	rec := KeyRecord{
		PublicID:    r.PublicID,
		DigestKeyID: r.PepperID,
		Digest:      append([]byte(nil), r.Digest...),
		TenantID:    r.Tenant,
		ProjectID:   r.Project,
		UserID:      r.User,
		Models:      append([]string(nil), r.Models...),
		Status:      r.Status,
		ExpiresAt:   r.ExpiresAt,
		AuthVersion: int64(r.AuthVersion),
	}
	for _, s := range r.Scopes {
		rec.Scopes = append(rec.Scopes, string(s))
	}
	if r.RevokedAt != nil {
		rec.RevokedAt = *r.RevokedAt
	}
	return rec
}

func authRecord(r KeyRecord) auth.KeyRecord {
	rec := auth.KeyRecord{
		PublicID:    r.PublicID,
		PepperID:    r.DigestKeyID,
		Digest:      append([]byte(nil), r.Digest...),
		Tenant:      r.TenantID,
		Project:     r.ProjectID,
		User:        r.UserID,
		Models:      append([]string(nil), r.Models...),
		Status:      r.Status,
		ExpiresAt:   r.ExpiresAt,
		AuthVersion: uint64(r.AuthVersion),
	}
	for _, s := range r.Scopes {
		rec.Scopes = append(rec.Scopes, s)
	}
	if !r.RevokedAt.IsZero() {
		revoked := r.RevokedAt
		rec.RevokedAt = &revoked
	}
	return rec
}

func authState(s AuthState) auth.AuthState {
	return auth.AuthState{Record: authRecord(s.Record), AuthVersion: s.AuthVersion}
}

func seamState(s auth.AuthState) AuthState {
	return AuthState{Record: seamRecord(s.Record), AuthVersion: s.AuthVersion}
}

// backendBridge adapts the test RevocationBackend onto auth.RevocationBackend.
type backendBridge struct{ backend RevocationBackend }

func (b backendBridge) Get(ctx context.Context, publicID string) (auth.AuthState, bool, error) {
	state, ok, err := b.backend.Get(ctx, publicID)
	if err != nil || !ok {
		return auth.AuthState{}, ok, err
	}
	return authState(state), true, nil
}

func (b backendBridge) Put(ctx context.Context, publicID string, state auth.AuthState, ttl time.Duration) error {
	return b.backend.Put(ctx, publicID, seamState(state), ttl)
}

func (b backendBridge) Delete(ctx context.Context, publicID string) error {
	return b.backend.Delete(ctx, publicID)
}

func (s *system) IssueKey(ctx context.Context, req IssueKeyRequest) (IssuedKey, error) {
	issued, err := s.impl.IssueKey(ctx, auth.IssueKeyRequest{
		Scope:     auth.Scope(req.Scope),
		UserID:    req.UserID,
		Scopes:    req.Scopes,
		Models:    req.Models,
		ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		return IssuedKey{}, mapErr(err)
	}
	return IssuedKey{Secret: issued.Value, Record: seamRecord(issued.Record)}, nil
}

func (s *system) RevokeKey(ctx context.Context, scope Scope, publicID string, at time.Time) error {
	return mapErr(s.impl.RevokeKey(ctx, auth.Scope(scope), publicID, at))
}

func (s *system) AuthenticatePublic(ctx context.Context, req *Request) (Principal, error) {
	p, err := s.impl.AuthenticatePublic(ctx, auth.Request{
		Method: req.Method, Path: req.Path, Header: req.Header, Query: req.Query, RemoteAddr: req.RemoteAddr,
	})
	if err != nil {
		return Principal{}, mapErr(err)
	}
	return Principal(p), nil
}

func (s *system) CheckPublicScope(p Principal, scope string) error {
	return mapErr(s.impl.CheckPublicScope(auth.Principal(p), scope))
}

func (s *system) AuthorizeModel(p Principal, model string) error {
	return mapErr(s.impl.AuthorizeModel(auth.Principal(p), model))
}

func (s *system) BootstrapAdmin(ctx context.Context, req BootstrapRequest) (AdminCredential, error) {
	cred, err := s.impl.BootstrapAdmin(ctx, auth.BootstrapRequest{AdminID: req.AdminID, Scopes: req.Scopes})
	if err != nil {
		return AdminCredential{}, mapErr(err)
	}
	return AdminCredential{Token: cred.Token, Principal: AdminPrincipal(cred.Principal)}, nil
}

func (s *system) IssueAdminToken(ctx context.Context, req IssueAdminTokenRequest) (AdminCredential, error) {
	cred, err := s.impl.IssueAdminToken(ctx, auth.IssueAdminTokenRequest{AdminID: req.AdminID, Scopes: req.Scopes, TTL: req.TTL})
	if err != nil {
		return AdminCredential{}, mapErr(err)
	}
	return AdminCredential{Token: cred.Token, Principal: AdminPrincipal(cred.Principal)}, nil
}

func (s *system) AuthenticateAdmin(ctx context.Context, req *Request) (AdminPrincipal, error) {
	p, err := s.impl.AuthenticateAdmin(ctx, auth.Request{
		Method: req.Method, Path: req.Path, Header: req.Header, Query: req.Query, RemoteAddr: req.RemoteAddr,
	})
	if err != nil {
		return AdminPrincipal{}, mapErr(err)
	}
	return AdminPrincipal(p), nil
}

func (s *system) CheckAdminScope(p AdminPrincipal, scope string) error {
	return mapErr(s.impl.CheckAdminScope(auth.AdminPrincipal(p), scope))
}

func (s *system) RevocationTTL() time.Duration { return s.ttl }

func (s *system) SecretWriter() SecretFileWriter { return secretFileWriter{} }

type secretFileWriter struct{}

func (secretFileWriter) WriteNew(path, secret string) error {
	if err := auth.WriteSecretExclusive(path, secret); err != nil {
		return fmt.Errorf("%w: %v", ErrSecretReuse, err)
	}
	return nil
}

type scopedStore struct {
	db     *postgres.DB
	store  *postgres.IdentityStore
	pepper auth.Pepper
}

func NewScopedStore(ctx context.Context, dsn string) (ScopedStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, ErrPGAdapterPending
	}
	pepperKey := make([]byte, 32)
	if _, err := rand.Read(pepperKey); err != nil {
		return nil, fmt.Errorf("identity test pepper unavailable")
	}
	db, err := postgres.Open(ctx, postgres.Config{DSN: dsn})
	if err != nil {
		return nil, fmt.Errorf("identity scoped store unavailable")
	}
	migrations, err := migrate.LoadEmbedded()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("identity migrations unavailable")
	}
	if err := (migrate.Runner{Pool: db.Pool()}).Run(ctx, migrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("identity schema unavailable")
	}
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("identity store unavailable")
	}
	return &scopedStore{db: db, store: store, pepper: auth.Pepper{ID: "identity-test-v1", Key: pepperKey}}, nil
}

func (s *scopedStore) SeedScope(ctx context.Context) (Scope, error) {
	tenantID := domain.TenantID(newScopedUUID())
	projectID := domain.ProjectID(newScopedUUID())
	if err := s.store.CreateTenant(ctx, domain.Tenant{ID: tenantID, Name: "identity-test-tenant", Currency: domain.Currency("USD"), Status: domain.TenantActive, Version: 1}); err != nil {
		return Scope{}, fmt.Errorf("seed tenant: %w", err)
	}
	if err := s.store.CreateProject(ctx, domain.Project{ID: projectID, Tenant: tenantID, Name: "identity-test-project", Version: 1}); err != nil {
		return Scope{}, fmt.Errorf("seed project: %w", err)
	}
	return Scope{TenantID: tenantID, ProjectID: projectID}, nil
}

func (s *scopedStore) SeedKey(ctx context.Context, scope Scope, seed KeySeed) (string, error) {
	if scope.TenantID.IsZero() || scope.ProjectID.IsZero() || seed.UserID.IsZero() || len(seed.Scopes) == 0 || len(seed.Models) == 0 {
		return "", ErrUnauthorized
	}
	now := time.Now().UTC()
	if !seed.ExpiresAt.After(now) {
		return "", ErrUnauthorized
	}
	if err := s.store.EnsureUser(ctx, scope.TenantID, seed.UserID, "identity-test-user"); err != nil {
		return "", fmt.Errorf("seed user: %w", err)
	}
	if err := s.store.EnsureProjectMember(ctx, scope.TenantID, scope.ProjectID, seed.UserID, "member"); err != nil {
		return "", fmt.Errorf("seed member: %w", err)
	}
	scopes := make([]domain.PermissionScope, 0, len(seed.Scopes))
	for _, value := range seed.Scopes {
		scopes = append(scopes, domain.PermissionScope(value))
	}
	issued, err := (auth.Issuer{Current: s.pepper}).Issue(scope.TenantID, scope.ProjectID, seed.UserID, scopes, seed.Models, now, seed.ExpiresAt.Sub(now))
	if err != nil {
		return "", fmt.Errorf("seed key: %w", err)
	}
	if err := s.store.CreateAPIKey(ctx, issued); err != nil {
		return "", fmt.Errorf("persist key: %w", err)
	}
	return issued.Record.PublicID, nil
}

func (s *scopedStore) AddMember(ctx context.Context, scope Scope, user domain.PrincipalID) error {
	if err := s.store.EnsureUser(ctx, scope.TenantID, user, "identity-test-user"); err != nil {
		return fmt.Errorf("add member user: %w", err)
	}
	return mapScopedStoreError(s.store.EnsureProjectMember(ctx, scope.TenantID, scope.ProjectID, user, "member"))
}

func (s *scopedStore) RequireMembership(ctx context.Context, scope Scope, user domain.PrincipalID) error {
	active, err := s.store.IsActiveMember(ctx, scope.TenantID, scope.ProjectID, user)
	if err != nil {
		return fmt.Errorf("membership lookup failed")
	}
	if !active {
		return ErrForbidden
	}
	return nil
}

func (s *scopedStore) GetKeyRecord(ctx context.Context, scope Scope, publicID string) (KeyRecord, error) {
	record, err := s.store.LookupAPIKey(ctx, scope.TenantID, scope.ProjectID, publicID)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) || errors.Is(err, auth.ErrRevoked) {
			return KeyRecord{}, ErrNotFound
		}
		return KeyRecord{}, fmt.Errorf("scoped key lookup failed")
	}
	return seamRecord(record), nil
}

func (s *scopedStore) ListKeyRecords(ctx context.Context, scope Scope, filter ListFilter) ([]KeyRecord, error) {
	records, err := s.store.ListAPIKeys(ctx, scope.TenantID, scope.ProjectID, filter.Status, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("scoped key list failed")
	}
	result := make([]KeyRecord, 0, len(records))
	for _, record := range records {
		result = append(result, seamRecord(record))
	}
	return result, nil
}

func (s *scopedStore) KeyStats(ctx context.Context, scope Scope) (Stats, error) {
	total, active, revoked, err := s.store.CountAPIKeys(ctx, scope.TenantID, scope.ProjectID)
	if err != nil {
		return Stats{}, fmt.Errorf("scoped key statistics failed")
	}
	return Stats{TotalKeys: total, ActiveKeys: active, RevokedKeys: revoked}, nil
}

func (s *scopedStore) Close() { s.db.Close() }

func newScopedUUID() domain.UUID {
	id, err := domain.NewUUID()
	if err != nil {
		panic("identity test UUID generation failed")
	}
	return id
}

func mapScopedStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, postgres.ErrNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("scoped store operation failed")
}

// Compile-time guard tying the adapter to the seam.
var _ System = (*system)(nil)
