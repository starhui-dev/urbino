package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"time"

	"example.com/urbino/internal/domain"
)

type AdminTokenRecord struct {
	ID          domain.UUID
	PrincipalID domain.PrincipalID
	PublicID    string
	Digest      []byte
	PepperID    string
	Scopes      []domain.PermissionScope
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	AuthVersion uint64
}
type IssuedAdminToken struct {
	Record AdminTokenRecord
	Secret string
	Value  string
}

func (i Issuer) IssueAdmin(scopes []domain.PermissionScope, now time.Time, ttl time.Duration) (IssuedAdminToken, error) {
	if i.Current.ID == "" || len(i.Current.Key) < 32 || ttl <= 0 || len(scopes) == 0 {
		return IssuedAdminToken{}, ErrInvalidCredential
	}
	for _, scope := range scopes {
		if !scope.Valid() {
			return IssuedAdminToken{}, fmt.Errorf("%w: invalid scope", ErrScopeDenied)
		}
	}
	id, err := domain.NewUUID()
	if err != nil {
		return IssuedAdminToken{}, fmt.Errorf("generate admin token id: %w", err)
	}
	publicBytes := make([]byte, 12)
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(publicBytes); err != nil {
		return IssuedAdminToken{}, fmt.Errorf("generate admin public id: %w", err)
	}
	if _, err := rand.Read(secretBytes); err != nil {
		return IssuedAdminToken{}, fmt.Errorf("generate admin secret: %w", err)
	}
	publicID := base64.RawURLEncoding.EncodeToString(publicBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	return IssuedAdminToken{
		Record: AdminTokenRecord{ID: id, PublicID: publicID, Digest: Digest(secret, i.Current.Key), PepperID: i.Current.ID, Scopes: append([]domain.PermissionScope(nil), scopes...), ExpiresAt: now.Add(ttl), AuthVersion: 1},
		Secret: secret,
		Value:  adminPrefix + publicID + "." + secret,
	}, nil
}

func VerifyAdmin(record AdminTokenRecord, secret string, pepper Pepper, now time.Time) error {
	if record.ID.IsZero() || record.PublicID == "" || record.AuthVersion == 0 || len(record.Digest) != 32 {
		return ErrInvalidCredential
	}
	if record.PepperID != pepper.ID || len(pepper.Key) < 32 {
		return ErrInvalidCredential
	}
	if !now.Before(record.ExpiresAt) {
		return ErrExpired
	}
	if record.RevokedAt != nil && !record.RevokedAt.After(now) {
		return ErrRevoked
	}
	got := Digest(secret, pepper.Key)
	if subtle.ConstantTimeCompare(record.Digest, got) != 1 {
		return ErrInvalidCredential
	}
	return nil
}

func (r AdminTokenRecord) Allows(scope domain.PermissionScope) bool {
	for _, candidate := range r.Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}
