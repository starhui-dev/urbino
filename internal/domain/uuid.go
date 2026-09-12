package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/starhui-dev/urbino/internal/clock"
)

// UUID is the canonical 128-bit identifier used by domain entities.
type UUID [16]byte

func (u UUID) IsZero() bool { return u == UUID{} }
func (u UUID) String() string {
	b := u
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]), hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]), hex.EncodeToString(b[10:16]))
}

func ParseUUID(s string) (UUID, error) {
	var u UUID
	s = strings.TrimSpace(s)
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return u, fmt.Errorf("invalid UUID")
	}
	clean := strings.ReplaceAll(s, "-", "")
	b, err := hex.DecodeString(clean)
	if err != nil || len(b) != 16 {
		return u, fmt.Errorf("invalid UUID")
	}
	copy(u[:], b)
	return u, nil
}

// NewUUIDv7 creates a UUIDv7 using the supplied clock and cryptographic reader.
// The reader is injectable solely for deterministic tests; production callers use crypto/rand.Reader.
func NewUUIDv7(c clock.Clock, r io.Reader) (UUID, error) {
	var u UUID
	if c == nil {
		return u, fmt.Errorf("nil clock")
	}
	if r == nil {
		r = rand.Reader
	}
	if _, err := io.ReadFull(r, u[:]); err != nil {
		return u, fmt.Errorf("uuid randomness: %w", err)
	}
	ms := uint64(c.Now().UTC().UnixMilli())
	// UUIDv7 timestamp occupies the first 48 bits, followed by version and random bits.
	u[0] = byte(ms >> 40)
	u[1] = byte(ms >> 32)
	u[2] = byte(ms >> 24)
	u[3] = byte(ms >> 16)
	u[4] = byte(ms >> 8)
	u[5] = byte(ms)
	u[6] = (u[6] & 0x0f) | 0x70
	u[8] = (u[8] & 0x3f) | 0x80
	return u, nil
}

func (u UUID) Version() byte        { return u[6] >> 4 }
func (u UUID) VariantRFC4122() bool { return u[8]&0xc0 == 0x80 }

// UTC normalizes a time value to UTC and rejects the zero time.
func UTC(t time.Time) (time.Time, error) {
	if t.IsZero() {
		return time.Time{}, fmt.Errorf("zero time")
	}
	return t.UTC(), nil
}

// UTCTime is a non-zero timestamp normalized to UTC at construction time.
type UTCTime struct{ time.Time }

func NewUTCTime(t time.Time) (UTCTime, error) {
	u, err := UTC(t)
	if err != nil {
		return UTCTime{}, err
	}
	return UTCTime{Time: u}, nil
}

func NowUTC(c clock.Clock) (UTCTime, error) {
	if c == nil {
		return UTCTime{}, fmt.Errorf("nil clock")
	}
	return NewUTCTime(c.Now())
}

// Entity IDs remain distinct at compile time even though they share UUID representation.
type TenantID UUID
type ProjectID UUID
type UserID UUID
type PrincipalID UUID
type RequestID UUID
type AttemptID UUID
type CredentialID UUID

func TenantIDFromUUID(u UUID) TenantID         { return TenantID(u) }
func ProjectIDFromUUID(u UUID) ProjectID       { return ProjectID(u) }
func UserIDFromUUID(u UUID) UserID             { return UserID(u) }
func PrincipalIDFromUUID(u UUID) PrincipalID   { return PrincipalID(u) }
func RequestIDFromUUID(u UUID) RequestID       { return RequestID(u) }
func AttemptIDFromUUID(u UUID) AttemptID       { return AttemptID(u) }
func CredentialIDFromUUID(u UUID) CredentialID { return CredentialID(u) }

func (id TenantID) String() string     { return UUID(id).String() }
func (id ProjectID) String() string    { return UUID(id).String() }
func (id UserID) String() string       { return UUID(id).String() }
func (id PrincipalID) String() string  { return UUID(id).String() }
func (id RequestID) String() string    { return UUID(id).String() }
func (id AttemptID) String() string    { return UUID(id).String() }
func (id CredentialID) String() string { return UUID(id).String() }
