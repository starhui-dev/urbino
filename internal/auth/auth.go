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
	"net"
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

// Key record lifecycle markers. Only a record with exactly recordStatusActive
// and no revocation timestamp may authorize anything.
const (
	recordStatusActive  = "active"
	recordStatusRevoked = "revoked"
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
	ErrLastAdminToken    = errors.New("cannot revoke the last available administrator token")
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

// ParsePublicCredential splits a gateway public credential after header
// parsing. It never accepts an administrator token.
func ParsePublicCredential(value string) (string, string, error) {
	return splitCredential(value, publicPrefix)
}

// ParseAdminCredential splits an administrator credential after header parsing.
func ParseAdminCredential(value string) (string, string, error) {
	return splitCredential(value, adminPrefix)
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
	if tenant.IsZero() || project.IsZero() || user.IsZero() || i.Current.ID == "" || len(i.Current.Key) < 32 || ttl <= 0 || len(scopes) == 0 || len(models) == 0 {
		return IssuedKey{}, ErrInvalidCredential
	}
	for _, scope := range scopes {
		if !isPublicScope(scope) {
			return IssuedKey{}, fmt.Errorf("%w: invalid public scope", ErrScopeDenied)
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
			Models: append([]string(nil), models...), Status: recordStatusActive, ExpiresAt: now.Add(ttl), AuthVersion: 1,
		},
		Secret: secret,
		Value:  publicPrefix + publicID + "." + secret,
	}, nil
}

func isPublicScope(scope domain.PermissionScope) bool {
	switch scope {
	case domain.ScopeModelsRead, domain.ScopeModelsInvoke, domain.ScopeUsageRead:
		return true
	default:
		return false
	}
}

func Digest(secret string, pepper []byte) []byte {
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte(secret))
	return mac.Sum(nil)
}

func Verify(record KeyRecord, presented string, pepper Pepper, now time.Time) error {
	if record.PublicID == "" || record.PepperID == "" || record.AuthVersion == 0 || record.Status != recordStatusActive || len(record.Digest) != sha256.Size || pepper.ID != record.PepperID || len(pepper.Key) < 32 {
		return ErrInvalidCredential
	}
	if !now.Before(record.ExpiresAt) {
		return ErrExpired
	}
	if record.RevokedAt != nil {
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

// VerifyRecord re-validates a stored key record without a presented secret. It
// backs Authorize, so it is a security boundary: any record that is not
// exactly active is rejected outright, and any revocation timestamp — past or
// future — revokes the record, because the marker alone means the key is no
// longer trusted.
func VerifyRecord(r KeyRecord, now time.Time) error {
	if r.Tenant.IsZero() || r.Project.IsZero() || r.User.IsZero() || r.PublicID == "" || r.AuthVersion == 0 || r.Status != recordStatusActive {
		return ErrInvalidCredential
	}
	if !now.Before(r.ExpiresAt) {
		return ErrExpired
	}
	if r.RevokedAt != nil {
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
	for key := range query {
		normalized := strings.ToLower(strings.TrimSpace(key))
		switch normalized {
		case "key", "api_key", "apikey", "api-key", "token", "access_token", "x-api-key", "x-goog-api-key":
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
		if !entry.state.CheckedAt.IsZero() && (entry.state.CheckedAt.After(now.Add(time.Second)) || now.Sub(entry.state.CheckedAt) > c.ttl) {
			return ErrCacheUnavailable
		}
		return checkRevocation(entry.state, authVersion)
	}
	state, err := c.store.Lookup(ctx, publicID)
	if err != nil {
		return ErrCacheUnavailable
	}
	if state.CheckedAt.IsZero() {
		state.CheckedAt = now
	} else if state.CheckedAt.After(now.Add(time.Second)) || now.Sub(state.CheckedAt) > c.ttl {
		return ErrCacheUnavailable
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

// maxFailureBuckets bounds how many identities the limiter tracks at once. A
// flood of distinct source addresses must never grow the map without limit.
const maxFailureBuckets = 4096

type FailureLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	max     int
	buckets map[string]failureBucket
}

type failureBucket struct {
	started  time.Time
	count    int
	inFlight int
}

// empty reports whether the bucket carries no state worth remembering.
func (b failureBucket) empty() bool { return b.count == 0 && b.inFlight == 0 }

func NewFailureLimiter(limit int, window time.Duration) (*FailureLimiter, error) {
	if limit <= 0 || window <= 0 {
		return nil, ErrRateLimited
	}
	return &FailureLimiter{limit: limit, window: window, max: maxFailureBuckets, buckets: make(map[string]failureBucket)}, nil
}

// Allow records one failed attempt immediately. It is retained for callers
// that already have a completed failure; request middleware should use Begin
// followed by RecordFailure or RecordSuccess so concurrent attempts reserve
// capacity atomically without charging successful requests.
func (l *FailureLimiter) Allow(identity string, now time.Time) bool {
	if l == nil || identity == "" {
		return false
	}
	identity = normalizeLimiterIdentity(identity)
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.currentLocked(identity, now)
	if bucket.count >= l.limit {
		return false
	}
	bucket.count++
	l.storeLocked(identity, bucket, now)
	return true
}

// Begin atomically reserves one authentication attempt. The reservation is
// converted into a failure or released on success by the matching method.
func (l *FailureLimiter) Begin(identity string, now time.Time) bool {
	if l == nil || identity == "" {
		return false
	}
	identity = normalizeLimiterIdentity(identity)
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.currentLocked(identity, now)
	if bucket.count+bucket.inFlight >= l.limit {
		return false
	}
	bucket.inFlight++
	l.storeLocked(identity, bucket, now)
	return true
}

// Check is kept as a compatibility read-only check. New request paths must
// use Begin because Check cannot reserve capacity against concurrent callers.
func (l *FailureLimiter) Check(identity string, now time.Time) bool {
	if l == nil || identity == "" {
		return false
	}
	identity = normalizeLimiterIdentity(identity)
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.currentLocked(identity, now)
	return bucket.count+bucket.inFlight < l.limit
}

// RecordFailure converts one in-flight reservation into a counted failure.
func (l *FailureLimiter) RecordFailure(identity string, now time.Time) {
	if l == nil || identity == "" {
		return
	}
	identity = normalizeLimiterIdentity(identity)
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.currentLocked(identity, now)
	if bucket.inFlight > 0 {
		bucket.inFlight--
	}
	if bucket.count < l.limit {
		bucket.count++
	}
	l.storeLocked(identity, bucket, now)
}

// RecordSuccess releases one in-flight reservation without consuming failure
// budget. It is safe when the request was rejected before Begin.
func (l *FailureLimiter) RecordSuccess(identity string, now time.Time) {
	if l == nil || identity == "" {
		return
	}
	identity = normalizeLimiterIdentity(identity)
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.currentLocked(identity, now)
	if bucket.inFlight > 0 {
		bucket.inFlight--
	}
	l.storeLocked(identity, bucket, now)
}

func (l *FailureLimiter) currentLocked(identity string, now time.Time) failureBucket {
	bucket := l.buckets[identity]
	if bucket.started.IsZero() || !now.Before(bucket.started.Add(l.window)) {
		return failureBucket{started: now}
	}
	return bucket
}

// storeLocked writes a bucket under the caller's lock and keeps the map
// bounded. Empty buckets are dropped rather than kept, windows that already
// expired are swept, and once the map is full the oldest surviving window is
// evicted. The identity being written is never the victim, and eviction stays
// inside the same critical section as Begin/RecordFailure/RecordSuccess so the
// reservation arithmetic remains atomic.
func (l *FailureLimiter) storeLocked(identity string, bucket failureBucket, now time.Time) {
	if bucket.empty() {
		delete(l.buckets, identity)
		return
	}
	if _, tracked := l.buckets[identity]; !tracked && len(l.buckets) >= l.capacityLocked() {
		l.evictLocked(now, identity)
	}
	l.buckets[identity] = bucket
}

func (l *FailureLimiter) capacityLocked() int {
	if l.max > 0 {
		return l.max
	}
	return maxFailureBuckets
}

// evictLocked makes room for one new identity: first every window that has
// already expired, then — only if the map is still at capacity — the oldest
// remaining window. Buckets holding in-flight reservations are evicted only
// when nothing else is available, so a saturated map still cannot grow without
// bound while normal traffic keeps its exact reservation accounting.
func (l *FailureLimiter) evictLocked(now time.Time, keep string) {
	oldest, oldestReserved := "", ""
	var oldestStarted, reservedStarted time.Time
	for key, bucket := range l.buckets {
		if key == keep {
			continue
		}
		if !now.Before(bucket.started.Add(l.window)) {
			delete(l.buckets, key)
			continue
		}
		if bucket.inFlight == 0 {
			if oldest == "" || bucket.started.Before(oldestStarted) {
				oldest, oldestStarted = key, bucket.started
			}
			continue
		}
		if oldestReserved == "" || bucket.started.Before(reservedStarted) {
			oldestReserved, reservedStarted = key, bucket.started
		}
	}
	if len(l.buckets) < l.capacityLocked() {
		return
	}
	if oldest == "" {
		oldest = oldestReserved
	}
	if oldest != "" {
		delete(l.buckets, oldest)
	}
}

func normalizeLimiterIdentity(identity string) string {
	if host, _, err := net.SplitHostPort(identity); err == nil && host != "" {
		return strings.Trim(host, "[]")
	}
	return identity
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
