package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// UUID is a 16-byte identifier with canonical text parsing and formatting.
type UUID [16]byte

func NewUUID() (UUID, error) {
	var id UUID
	if _, err := rand.Read(id[:]); err != nil {
		return UUID{}, fmt.Errorf("generate uuid: %w", err)
	}
	// RFC 9562 UUIDv7: 48-bit Unix millisecond timestamp followed by
	// twelve random bits, the version nibble, and a variant-constrained tail.
	ms := time.Now().UnixMilli()
	if ms < 0 || ms > 0xffffffffffff {
		return UUID{}, fmt.Errorf("generate uuid: timestamp out of range")
	}
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)
	id[6] = (id[6] & 0x0f) | 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	return id, nil
}

func ParseUUID(value string) (UUID, error) {
	var id UUID
	value = strings.TrimSpace(value)
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return id, fmt.Errorf("invalid uuid")
	}
	decoded, err := hex.DecodeString(value[:8] + value[9:13] + value[14:18] + value[19:23] + value[24:])
	if err != nil || len(decoded) != len(id) {
		return id, fmt.Errorf("invalid uuid")
	}
	copy(id[:], decoded)
	return id, nil
}

func (id UUID) String() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
}

// MarshalText makes UUID fields stable strings in JSON and other text formats.
func (id UUID) MarshalText() ([]byte, error) { return []byte(id.String()), nil }

func (id UUID) IsZero() bool { return id == UUID{} }

// UTC is a timestamp normalized to UTC at construction time.
type UTC struct{ time.Time }

func NewUTC(t time.Time) UTC { return UTC{Time: t.UTC()} }
func (t UTC) Valid() bool    { return !t.Time.IsZero() && t.Time.Location() == time.UTC }

// Scope identifiers keep tenant isolation explicit at type boundaries.
type TenantID UUID
type ProjectID UUID
type PrincipalID UUID
type RequestID UUID
type AttemptID UUID

func (id TenantID) String() string                  { return UUID(id).String() }
func (id ProjectID) String() string                 { return UUID(id).String() }
func (id PrincipalID) String() string               { return UUID(id).String() }
func (id RequestID) String() string                 { return UUID(id).String() }
func (id AttemptID) String() string                 { return UUID(id).String() }
func (id TenantID) MarshalText() ([]byte, error)    { return []byte(id.String()), nil }
func (id ProjectID) MarshalText() ([]byte, error)   { return []byte(id.String()), nil }
func (id PrincipalID) MarshalText() ([]byte, error) { return []byte(id.String()), nil }
func (id RequestID) MarshalText() ([]byte, error)   { return []byte(id.String()), nil }
func (id AttemptID) MarshalText() ([]byte, error)   { return []byte(id.String()), nil }

func (id TenantID) IsZero() bool    { return UUID(id).IsZero() }
func (id ProjectID) IsZero() bool   { return UUID(id).IsZero() }
func (id PrincipalID) IsZero() bool { return UUID(id).IsZero() }
func (id RequestID) IsZero() bool   { return UUID(id).IsZero() }
func (id AttemptID) IsZero() bool   { return UUID(id).IsZero() }

func ParseTenantID(value string) (TenantID, error) {
	id, err := ParseUUID(value)
	return TenantID(id), err
}

func ParseProjectID(value string) (ProjectID, error) {
	id, err := ParseUUID(value)
	return ProjectID(id), err
}

func ParsePrincipalID(value string) (PrincipalID, error) {
	id, err := ParseUUID(value)
	return PrincipalID(id), err
}

func ParseRequestID(value string) (RequestID, error) {
	id, err := ParseUUID(value)
	return RequestID(id), err
}

func ParseAttemptID(value string) (AttemptID, error) {
	id, err := ParseUUID(value)
	return AttemptID(id), err
}
