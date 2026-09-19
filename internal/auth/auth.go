// Package auth contains the credential and authorization primitives shared by
// the public and management listeners. It deliberately does not depend on a
// database, cache product, or HTTP router.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"example.com/urbino/internal/domain"
)

const (
	publicPrefix = "gw_live_"
	adminPrefix  = "gw_admin_"
	maxCacheTTL  = 5 * time.Second
)

var (
	ErrInvalidCredential = errors.New("invalid credential")
	ErrExpired           = errors.New("credential expired")
	ErrRevoked           = errors.New("credential revoked")
	ErrVersionMismatch   = errors.New("credential version changed")
	ErrScopeDenied       = errors.New("credential scope denied")
	ErrHeaderConflict    = errors.New("conflicting authentication headers")
	ErrQueryCredential   = errors.New("query credentials are not accepted")
	ErrCacheUnavailable  = errors.New("authentication cache unavailable")
	ErrRateLimited       = errors.New("authentication rate limited")
)

type ListenerMode uint8

const (
	PublicListener ListenerMode = iota + 1
	AdminListener
)

type CredentialKind uint8

const (
	GatewayCredential CredentialKind = iota + 1
	AdminCredentialKind
)

type HeaderCredential struct {
	Kind  CredentialKind
	Value string
}

type Pepper struct {
	ID  string
	Key []byte
}

type Issuer struct {
	Current Pepper
}

type KeyRecord struct {
	ID          domain.UUID
	PublicID    string
	Tenant      domain.TenantID
	Project     domain.ProjectID
	User        domain.PrincipalID
	Digest      []byte
	PepperID    string
	Scopes      []string
	Models      []string
	Status      string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	AuthVersion uint64
}

type IssuedKey struct {
	Record KeyRecord
	Secret string
	Value  string
}

func (i Issuer) Issue(tenant domain.TenantID, project domain.ProjectID, user domain.PrincipalID, scopes []domain.PermissionScope, models []string, now time.Time, ttl time.Duration) (IssuedKey, error) {
	if tenant.IsZero() || project.IsZero() || user.IsZero() || i.Current.ID == "" || len(i.Current.Key) < 32 || ttl <= 0 {
		return IssuedKey{}, ErrInvalidCredential
	}
	for _, scope := range scopes {
		if !scope.Valid() {
			return IssuedKey{}, fmt.Errorf("%w: invalid scope", ErrScopeDenied)
		}
	}
	id, err := domain.NewUUID()
	if err != nil {
		return IssuedKey{}, fmt.Errorf("generate key id: %w", err)
	}
	publicBytes := make([]byte, 12)
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(publicBytes); err != nil {
		return IssuedKey{}, fmt.Errorf("generate public id: %w", err)
	}
	if _, err := rand.Read(secretBytes); err != nil {
		return IssuedKey{}, fmt.Errorf("generate secret: %w", err)
	}
	publicID := base64.RawURLEncoding.EncodeToString(publicBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	digest := Digest(secret, i.Current.Key)
	stringScopes := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		stringScopes = append(stringScopes, string(scope))
	}
	return IssuedKey{
		Record: KeyRecord{
			ID: id, PublicID: publicID, Tenant: tenant, Project: project, User: user,
			Digest: digest, PepperID: i.Current.ID, Scopes: stringScopes,
			Models: append([]string(nil), models...), Status: "active", ExpiresAt: now.Add(ttl), AuthVersion: 1,
		},
		Secret: secret,
		Value:  publicPrefix + publicID + "." + secret,
	}, nil
}

func Digest(secret string, pepper []byte) []byte {
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte(secret))
	return mac.Sum(nil)
}

func Verify(record KeyRecord, presented string, pepper Pepper, now time.Time) error {
	if record.PublicID == "" || record.PepperID == "" || record.AuthVersion == 0 || len(record.Digest) != sha256.Size || pepper.ID != record.PepperID || len(pepper.Key) < 32 {
		return ErrInvalidCredential
	}
	if !now.Before(record.ExpiresAt) {
		return ErrExpired
	}
	if record.RevokedAt != nil && !record.RevokedAt.After(now) {
		return ErrRevoked
	}
	got := Digest(presented, pepper.Key)
	if subtle.ConstantTimeCompare(record.Digest, got) != 1 {
		return ErrInvalidCredential
	}
	return nil
}

func (r KeyRecord) AllowsScope(scope domain.PermissionScope) bool {
	for _, candidate := range r.Scopes {
		if candidate == string(scope) {
			return true
		}
	}
	return false

}

func (r KeyRecord) AllowsModel(model string) bool {
	for _, allowed := range r.Models {
		if allowed == model {
			return true
		}
	}
	return false
}

func (r KeyRecord) Authorize(model string, scope domain.PermissionScope, now time.Time) error {
	if err := VerifyRecord(r, now); err != nil {
		return err
	}
	if !r.AllowsScope(scope) || !r.AllowsModel(model) {
		return ErrScopeDenied
	}
	return nil
}

func VerifyRecord(r KeyRecord, now time.Time) error {
	if r.Tenant.IsZero() || r.Project.IsZero() || r.User.IsZero() || r.PublicID == "" || r.AuthVersion == 0 {
		return ErrInvalidCredential
	}
	if !now.Before(r.ExpiresAt) {
		return ErrExpired
	}
	if r.RevokedAt != nil && !r.RevokedAt.After(now) {
		return ErrRevoked
	}
	return nil
}

func ParseHeaders(headers http.Header, query url.Values, mode ListenerMode) (HeaderCredential, error) {
	if hasQueryCredential(query) {
		return HeaderCredential{}, ErrQueryCredential
	}
	var candidates []HeaderCredential
	authorization := headerValues(headers, "Authorization")
	if len(authorization) > 1 {
		return HeaderCredential{}, ErrHeaderConflict
	}
	if len(authorization) == 1 && strings.TrimSpace(authorization[0]) != "" {
		parts := strings.Fields(strings.TrimSpace(authorization[0]))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") || parts[1] == "" {
			return HeaderCredential{}, ErrInvalidCredential
		}
		kind := GatewayCredential
		if strings.HasPrefix(parts[1], adminPrefix) {
			kind = AdminCredentialKind
		}
		candidates = append(candidates, HeaderCredential{Kind: kind, Value: parts[1]})
	}
	for _, name := range []string{"X-API-Key", "X-Goog-Api-Key"} {
		values := headerValues(headers, name)
		if len(values) > 1 {
			return HeaderCredential{}, ErrHeaderConflict
		}
		if len(values) == 1 && strings.TrimSpace(values[0]) != "" {
			candidates = append(candidates, HeaderCredential{Kind: GatewayCredential, Value: strings.TrimSpace(values[0])})
		}
	}
	if len(candidates) != 1 {
		return HeaderCredential{}, ErrHeaderConflict
	}
	credential := candidates[0]
	if mode == PublicListener && credential.Kind != GatewayCredential {
		return HeaderCredential{}, ErrInvalidCredential
	}
	if mode == AdminListener && credential.Kind != AdminCredentialKind {
		return HeaderCredential{}, ErrInvalidCredential
	}
	if credential.Kind == GatewayCredential && !strings.HasPrefix(credential.Value, publicPrefix) {
		return HeaderCredential{}, ErrInvalidCredential
	}
	if credential.Kind == AdminCredentialKind && !strings.HasPrefix(credential.Value, adminPrefix) {
		return HeaderCredential{}, ErrInvalidCredential
	}
	return credential, nil
}

func headerValues(headers http.Header, name string) []string {
	var result []string
	for key, values := range headers {
		if strings.EqualFold(key, name) {
			result = append(result, values...)
		}
	}
	return result
}

func hasQueryCredential(query url.Values) bool {
	for _, key := range []string{"key", "api_key", "apikey", "api-key", "token", "access_token", "x-api-key", "x-goog-api-key"} {
		if _, ok := query[key]; ok {
			return true
		}
	}
	return false
}

type RevocationState struct {
	Revoked     bool
	AuthVersion uint64
	CheckedAt   time.Time
}

type RevocationStore interface {
	Lookup(context.Context, string) (RevocationState, error)
}

type revocationEntry struct {
	state     RevocationState
	expiresAt time.Time
}

type RevocationCache struct {
	mu     sync.Mutex
	store  RevocationStore
	ttl    time.Duration
	values map[string]revocationEntry
}

func NewRevocationCache(store RevocationStore, ttl time.Duration) (*RevocationCache, error) {
	if store == nil || ttl <= 0 || ttl > maxCacheTTL {
		return nil, ErrCacheUnavailable
	}
	return &RevocationCache{store: store, ttl: ttl, values: make(map[string]revocationEntry)}, nil
}

func (c *RevocationCache) Check(ctx context.Context, publicID string, authVersion uint64, now time.Time) error {
	if c == nil || c.store == nil || publicID == "" || authVersion == 0 {
		return ErrCacheUnavailable
	}
	c.mu.Lock()
	entry, ok := c.values[publicID]
	c.mu.Unlock()
	if ok && now.Before(entry.expiresAt) {
		return checkRevocation(entry.state, authVersion)
	}
	state, err := c.store.Lookup(ctx, publicID)
	if err != nil {
		return ErrCacheUnavailable
	}
	if state.CheckedAt.IsZero() {
		state.CheckedAt = now
	}
	c.mu.Lock()
	c.values[publicID] = revocationEntry{state: state, expiresAt: now.Add(c.ttl)}
	c.mu.Unlock()
	return checkRevocation(state, authVersion)
}

func (c *RevocationCache) Invalidate(publicID string) {
	c.mu.Lock()
	delete(c.values, publicID)
	c.mu.Unlock()
}

func checkRevocation(state RevocationState, authVersion uint64) error {
	if state.Revoked {
		return ErrRevoked
	}
	if state.AuthVersion != authVersion {
		return ErrVersionMismatch
	}
	return nil
}

type FailureLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]failureBucket
}

type failureBucket struct {
	started time.Time
	count   int
}

func NewFailureLimiter(limit int, window time.Duration) (*FailureLimiter, error) {
	if limit <= 0 || window <= 0 {
		return nil, ErrRateLimited
	}
	return &FailureLimiter{limit: limit, window: window, buckets: make(map[string]failureBucket)}, nil
}

func (l *FailureLimiter) Allow(identity string, now time.Time) bool {
	if l == nil || identity == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.buckets[identity]
	if bucket.started.IsZero() || !now.Before(bucket.started.Add(l.window)) {
		l.buckets[identity] = failureBucket{started: now, count: 1}
		return true
	}
	if bucket.count >= l.limit {
		return false
	}
	bucket.count++
	l.buckets[identity] = bucket
	return true
}

// Check reports whether another authentication attempt may be evaluated.
// It does not consume quota; callers must record only failed attempts.
func (l *FailureLimiter) Check(identity string, now time.Time) bool {
	if l == nil || identity == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.buckets[identity]
	return bucket.started.IsZero() || !now.Before(bucket.started.Add(l.window)) || bucket.count < l.limit
}

// RecordFailure consumes one failure slot for the identity.
func (l *FailureLimiter) RecordFailure(identity string, now time.Time) {
	if l == nil || identity == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.buckets[identity]
	if bucket.started.IsZero() || !now.Before(bucket.started.Add(l.window)) {
		l.buckets[identity] = failureBucket{started: now, count: 1}
		return
	}
	if bucket.count < l.limit {
		bucket.count++
	}
	l.buckets[identity] = bucket
}

func WriteSecretExclusive(path, secret string) error {
	if strings.TrimSpace(path) == "" || secret == "" {
		return errors.New("secret output path and secret are required")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("secret output file already exists or is unavailable")
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.WriteString(secret + "\n"); err != nil {
		return errors.New("secret output failed")
	}
	if err := file.Sync(); err != nil {
		return errors.New("secret output failed")
	}
	ok = true
	return nil
}

func ReadPepperFile(path, id string) (Pepper, error) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(id) == "" {
		return Pepper{}, ErrInvalidCredential
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return Pepper{}, ErrInvalidCredential
	}
	file := os.NewFile(uintptr(fd), "auth.pepper")
	if file == nil {
		_ = syscall.Close(fd)
		return Pepper{}, ErrInvalidCredential
	}
	defer file.Close()
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil || stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		return Pepper{}, ErrInvalidCredential
	}
	value, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return Pepper{}, ErrInvalidCredential
	}
	trimmed := strings.TrimSpace(string(value))
	if len(trimmed) < 32 || len(trimmed) > 4096 {
		return Pepper{}, ErrInvalidCredential
	}
	return Pepper{ID: id, Key: []byte(trimmed)}, nil
}
