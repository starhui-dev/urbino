package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"example.com/urbino/internal/clock"
	"example.com/urbino/internal/domain"
)

// Administrator credential bounds. Bootstrap uses the same bounded lifetime
// and fixed management authority as the PostgreSQL installation path.
const (
	bootstrapAdminTokenTTL = 24 * time.Hour
	maxAdminTokenTTL       = 24 * time.Hour
)

var bootstrapAdminScopes = []domain.PermissionScope{
	domain.ScopeTenantsRead, domain.ScopeTenantsWrite,
	domain.ScopeKeysRead, domain.ScopeKeysWrite,
}

type Scope struct {
	TenantID  domain.TenantID
	ProjectID domain.ProjectID
}

type IssueKeyRequest struct {
	Scope     Scope
	UserID    domain.PrincipalID
	Scopes    []string
	Models    []string
	ExpiresAt time.Time
}

type Principal struct {
	TenantID    domain.TenantID
	ProjectID   domain.ProjectID
	UserID      domain.PrincipalID
	KeyID       string
	Scopes      []string
	Models      []string
	AuthVersion int64
}

type AdminPrincipal struct {
	AdminID domain.PrincipalID
	Scopes  []string
}

type AdminCredential struct {
	Token     string
	Principal AdminPrincipal
}

// BootstrapRequest creates the first administrator of an installation. Scopes
// is advisory only: the bootstrap credential always carries the fixed minimal
// bootstrap scope set, so request content can never grant write authority.
type BootstrapRequest struct {
	AdminID domain.PrincipalID
	Scopes  []string
}

// IssueAdminTokenRequest issues an additional administrator token. Scopes must
// already be held by the principal and TTL must not exceed maxAdminTokenTTL.
type IssueAdminTokenRequest struct {
	AdminID domain.PrincipalID
	Scopes  []string
	TTL     time.Duration
}

type Request struct {
	Method     string
	Path       string
	Header     http.Header
	Query      url.Values
	RemoteAddr string
}

type AuthState struct {
	Record      KeyRecord
	AuthVersion int64
}

type RevocationBackend interface {
	Get(context.Context, string) (AuthState, bool, error)
	Put(context.Context, string, AuthState, time.Duration) error
	Delete(context.Context, string) error
}

type Options struct {
	Clock          clock.Clock
	Peppers        map[string][]byte
	ActivePepperID string
	Backend        RevocationBackend
	RevocationTTL  time.Duration
	Logger         *slog.Logger
}

type Service struct {
	mu           sync.RWMutex
	clock        clock.Clock
	peppers      map[string]Pepper
	activeID     string
	backend      RevocationBackend
	logger       *slog.Logger
	ttl          time.Duration
	keys         map[string]KeyRecord
	admins       map[string]AdminTokenRecord
	principals   map[domain.PrincipalID]AdminPrincipal
	bootstrapped bool
}

func New(_ context.Context, options Options) (*Service, time.Duration, error) {
	if options.Clock == nil {
		options.Clock = clock.Real{}
	}
	if options.Backend == nil || options.ActivePepperID == "" {
		return nil, 0, ErrCacheUnavailable
	}
	peppers := make(map[string]Pepper, len(options.Peppers))
	for id, key := range options.Peppers {
		if id == "" || len(key) < 32 {
			return nil, 0, ErrInvalidCredential
		}
		peppers[id] = Pepper{ID: id, Key: append([]byte(nil), key...)}
	}
	active, ok := peppers[options.ActivePepperID]
	if !ok || len(active.Key) < 32 {
		return nil, 0, ErrInvalidCredential
	}
	ttl := 5 * time.Second
	if options.RevocationTTL > 0 && options.RevocationTTL <= maxCacheTTL {
		ttl = options.RevocationTTL
	}
	return &Service{
		clock: options.Clock, peppers: peppers, activeID: options.ActivePepperID, backend: options.Backend,
		logger: options.Logger, ttl: ttl, keys: make(map[string]KeyRecord),
		admins: make(map[string]AdminTokenRecord), principals: make(map[domain.PrincipalID]AdminPrincipal),
	}, ttl, nil
}

func (s *Service) IssueKey(ctx context.Context, req IssueKeyRequest) (IssuedKey, error) {
	if s == nil || req.Scope.TenantID.IsZero() || req.Scope.ProjectID.IsZero() || req.UserID.IsZero() || len(req.Scopes) == 0 || len(req.Models) == 0 || !req.ExpiresAt.After(s.clock.Now()) {
		return IssuedKey{}, ErrInvalidCredential
	}
	scopes := make([]domain.PermissionScope, 0, len(req.Scopes))
	for _, scope := range req.Scopes {
		candidate := domain.PermissionScope(scope)
		if !candidate.Valid() {
			return IssuedKey{}, ErrScopeDenied
		}
		scopes = append(scopes, candidate)
	}
	issued, err := (Issuer{Current: s.peppers[s.activeID]}).Issue(req.Scope.TenantID, req.Scope.ProjectID, req.UserID, scopes, req.Models, s.clock.Now(), req.ExpiresAt.Sub(s.clock.Now()))
	if err != nil {
		return IssuedKey{}, err
	}
	s.mu.Lock()
	s.keys[issued.Record.PublicID] = issued.Record
	s.mu.Unlock()
	if err := s.backend.Put(ctx, issued.Record.PublicID, AuthState{Record: issued.Record, AuthVersion: int64(issued.Record.AuthVersion)}, s.ttl); err != nil {
		return IssuedKey{}, ErrCacheUnavailable
	}
	return issued, nil
}

func (s *Service) RevokeKey(ctx context.Context, scope Scope, publicID string, at time.Time) error {
	s.mu.RLock()
	record, ok := s.keys[publicID]
	s.mu.RUnlock()
	if !ok {
		state, found, err := s.backend.Get(ctx, publicID)
		if err != nil {
			return ErrCacheUnavailable
		}
		if !found {
			return ErrNotFound
		}
		record, ok = state.Record, true
	}
	if !ok || record.Tenant != scope.TenantID || record.Project != scope.ProjectID {
		return ErrNotFound
	}
	record.Status = recordStatusRevoked
	record.RevokedAt = &at
	record.AuthVersion++
	s.mu.Lock()
	s.keys[publicID] = record
	s.mu.Unlock()
	if err := s.backend.Delete(ctx, publicID); err != nil {
		return ErrCacheUnavailable
	}
	if err := s.backend.Put(ctx, publicID, AuthState{Record: record, AuthVersion: int64(record.AuthVersion)}, s.ttl); err != nil {
		return ErrCacheUnavailable
	}
	return nil
}

func (s *Service) AuthenticatePublic(ctx context.Context, req Request) (Principal, error) {
	credential, err := ParseHeaders(req.Header, req.Query, PublicListener)
	if err != nil {
		return Principal{}, err
	}
	publicID, secret, err := splitCredential(credential.Value, publicPrefix)
	if err != nil {
		return Principal{}, ErrInvalidCredential
	}
	s.mu.RLock()
	authoritative, ok := s.keys[publicID]
	s.mu.RUnlock()
	if !ok {
		return Principal{}, ErrInvalidCredential
	}
	state, found, err := s.backend.Get(ctx, publicID)
	if err != nil {
		return Principal{}, ErrCacheUnavailable
	}
	if !found {
		if authoritative.RevokedAt != nil || authoritative.Status == recordStatusRevoked {
			return Principal{}, ErrRevoked
		}
		return Principal{}, ErrCacheUnavailable
	}
	if state.Record.Status == recordStatusRevoked {
		return Principal{}, ErrRevoked
	}
	if state.AuthVersion != int64(authoritative.AuthVersion) {
		if authoritative.RevokedAt != nil {
			return Principal{}, ErrRevoked
		}
		return Principal{}, ErrVersionMismatch
	}
	pepper, ok := s.peppers[authoritative.PepperID]
	if !ok {
		return Principal{}, ErrInvalidCredential
	}
	if err := Verify(authoritative, secret, pepper, s.clock.Now()); err != nil {
		return Principal{}, err
	}
	if authoritative.RevokedAt != nil || authoritative.Status == recordStatusRevoked {
		return Principal{}, ErrRevoked
	}
	if s.logger != nil {
		s.logger.Debug("public authentication", "key_id", publicID, "remote_addr", req.RemoteAddr)
	}
	return Principal{TenantID: authoritative.Tenant, ProjectID: authoritative.Project, UserID: authoritative.User, KeyID: publicID, Scopes: append([]string(nil), authoritative.Scopes...), Models: append([]string(nil), authoritative.Models...), AuthVersion: int64(authoritative.AuthVersion)}, nil
}

func (s *Service) CheckPublicScope(principal Principal, scope string) error {
	if !containsScope(principal.Scopes, scope) {
		return ErrScopeDenied
	}
	return nil
}

func (s *Service) AuthorizeModel(principal Principal, model string) error {
	if !containsScope(principal.Models, model) {
		return ErrModelDenied
	}
	return nil
}

// BootstrapAdmin creates the one and only bootstrap administrator. The issued
// credential always carries the fixed minimal bootstrap scope set and the
// bounded bootstrap lifetime; BootstrapRequest.Scopes is advisory only, so no
// request content can raise the authority of the first administrator.
func (s *Service) BootstrapAdmin(_ context.Context, req BootstrapRequest) (AdminCredential, error) {
	if s == nil {
		return AdminCredential{}, ErrInvalidCredential
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bootstrapped {
		return AdminCredential{}, ErrAlreadyBootstrapped
	}
	if req.AdminID.IsZero() {
		return AdminCredential{}, ErrInvalidCredential
	}
	issued, err := (Issuer{Current: s.peppers[s.activeID]}).IssueAdmin(bootstrapAdminScopes, s.clock.Now(), bootstrapAdminTokenTTL)
	if err != nil {
		return AdminCredential{}, err
	}
	principal := AdminPrincipal{AdminID: req.AdminID, Scopes: toScopeStrings(bootstrapAdminScopes)}
	issued.Record.PrincipalID = req.AdminID
	s.bootstrapped = true
	s.principals[req.AdminID] = principal
	s.admins[issued.Record.PublicID] = issued.Record
	return AdminCredential{Token: issued.Value, Principal: principal}, nil
}

// IssueAdminToken issues an additional token for an existing administrator.
// The requested scopes must be valid, non-empty and already held by the
// principal — a token can narrow authority but never widen it — and the
// requested lifetime must fall inside (0, maxAdminTokenTTL].
func (s *Service) IssueAdminToken(_ context.Context, req IssueAdminTokenRequest) (AdminCredential, error) {
	if s == nil || req.TTL <= 0 || req.TTL > maxAdminTokenTTL || len(req.Scopes) == 0 {
		return AdminCredential{}, ErrInvalidCredential
	}
	requested := make([]domain.PermissionScope, 0, len(req.Scopes))
	seen := make(map[domain.PermissionScope]struct{}, len(req.Scopes))
	for _, scope := range req.Scopes {
		candidate := domain.PermissionScope(scope)
		if !candidate.Valid() {
			return AdminCredential{}, ErrScopeDenied
		}
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		requested = append(requested, candidate)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	principal, ok := s.principals[req.AdminID]
	if !ok {
		return AdminCredential{}, ErrNotFound
	}
	for _, scope := range requested {
		if !containsScope(principal.Scopes, string(scope)) {
			return AdminCredential{}, ErrScopeDenied
		}
	}
	issued, err := (Issuer{Current: s.peppers[s.activeID]}).IssueAdmin(requested, s.clock.Now(), req.TTL)
	if err != nil {
		return AdminCredential{}, err
	}
	issued.Record.PrincipalID = req.AdminID
	s.admins[issued.Record.PublicID] = issued.Record
	return AdminCredential{Token: issued.Value, Principal: AdminPrincipal{AdminID: req.AdminID, Scopes: toScopeStrings(requested)}}, nil
}

// RevokeAdminToken revokes one administrator credential in place. The owning
// principal must be named, so one administrator cannot revoke another's token;
// the revocation timestamp is set whatever value is supplied, because
// verification fails closed on the marker itself. Repeated revocation is
// refused with ErrRevoked and the revoked record is kept as evidence.
func (s *Service) RevokeAdminToken(_ context.Context, adminID domain.PrincipalID, publicID string, at time.Time) error {
	if s == nil || adminID.IsZero() || publicID == "" {
		return ErrInvalidCredential
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.admins[publicID]
	if !ok || record.PrincipalID != adminID {
		return ErrNotFound
	}
	if record.RevokedAt != nil {
		return ErrRevoked
	}
	available := 0
	for _, candidate := range s.admins {
		if candidate.RevokedAt == nil && candidate.ExpiresAt.After(s.clock.Now()) {
			available++
		}
	}
	if available <= 1 {
		return ErrLastAdminToken
	}
	record.RevokedAt = &at
	record.AuthVersion++
	s.admins[publicID] = record
	return nil
}

func (s *Service) AuthenticateAdmin(_ context.Context, req Request) (AdminPrincipal, error) {
	credential, err := ParseHeaders(req.Header, req.Query, AdminListener)
	if err != nil {
		return AdminPrincipal{}, err
	}
	publicID, secret, err := splitCredential(credential.Value, adminPrefix)
	if err != nil {
		return AdminPrincipal{}, ErrInvalidCredential
	}
	s.mu.RLock()
	record, ok := s.admins[publicID]
	s.mu.RUnlock()
	if !ok {
		return AdminPrincipal{}, ErrInvalidCredential
	}
	pepper, ok := s.peppers[record.PepperID]
	if !ok {
		return AdminPrincipal{}, ErrInvalidCredential
	}
	if err := VerifyAdmin(record, secret, pepper, s.clock.Now()); err != nil {
		return AdminPrincipal{}, err
	}
	return AdminPrincipal{AdminID: record.PrincipalID, Scopes: toScopeStrings(record.Scopes)}, nil
}

func (s *Service) CheckAdminScope(principal AdminPrincipal, scope string) error {
	if !containsScope(principal.Scopes, scope) {
		return ErrScopeDenied
	}
	return nil
}

func (s *Service) RevocationTTL() time.Duration {
	if s == nil {
		return 0
	}
	return s.ttl
}

func splitCredential(value, prefix string) (string, string, error) {
	if !strings.HasPrefix(value, prefix) {
		return "", "", ErrInvalidCredential
	}
	parts := strings.Split(strings.TrimPrefix(value, prefix), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", ErrInvalidCredential
	}
	return parts[0], parts[1], nil
}

func toScopeStrings(scopes []domain.PermissionScope) []string {
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		result = append(result, string(scope))
	}
	return result
}

func containsScope(scopes []string, scope string) bool {
	for _, candidate := range scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func IsExpired(err error) bool              { return errors.Is(err, ErrExpired) }
func IsRevoked(err error) bool              { return errors.Is(err, ErrRevoked) }
func IsStaleVersion(err error) bool         { return errors.Is(err, ErrVersionMismatch) }
func IsAmbiguousCredentials(err error) bool { return errors.Is(err, ErrHeaderConflict) }
func IsQueryKey(err error) bool             { return errors.Is(err, ErrQueryCredential) }
func IsScopeDenied(err error) bool          { return errors.Is(err, ErrScopeDenied) }
func IsModelDenied(err error) bool          { return errors.Is(err, ErrModelDenied) }
func IsCacheUnavailable(err error) bool     { return errors.Is(err, ErrCacheUnavailable) }
func IsAlreadyBootstrapped(err error) bool  { return errors.Is(err, ErrAlreadyBootstrapped) }
func IsNotFound(err error) bool             { return errors.Is(err, ErrNotFound) }
func IsForbidden(err error) bool {
	return errors.Is(err, ErrScopeDenied) || errors.Is(err, ErrModelDenied)
}

var (
	ErrModelDenied         = errors.New("model denied")
	ErrNotFound            = errors.New("resource not found")
	ErrAlreadyBootstrapped = errors.New("administrator bootstrap already completed")
)
