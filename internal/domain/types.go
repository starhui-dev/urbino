package domain

import (
	"fmt"
	"math"
)

type PermissionScope string

const (
	ScopeTenantsRead  PermissionScope = "tenants:read"
	ScopeTenantsWrite PermissionScope = "tenants:write"
	ScopeModelsRead   PermissionScope = "models:read"
	ScopeUsageRead    PermissionScope = "usage:read"
)

func (s PermissionScope) Valid() bool {
	switch s {
	case ScopeTenantsRead, ScopeTenantsWrite, ScopeModelsRead, ScopeUsageRead:
		return true
	default:
		return false
	}
}

type CapabilityName string

const (
	CapabilityAdminAPI   CapabilityName = "admin_api"
	CapabilityChat       CapabilityName = "chat_completions"
	CapabilityResponses  CapabilityName = "responses"
	CapabilityEmbeddings CapabilityName = "embeddings"
)

func (n CapabilityName) Valid() bool {
	switch n {
	case CapabilityAdminAPI, CapabilityChat, CapabilityResponses, CapabilityEmbeddings:
		return true
	default:
		return false
	}
}

type CapabilityDescriptor struct {
	Name    CapabilityName
	Version string
	Enabled bool
	Scopes  []PermissionScope
}

func (c CapabilityDescriptor) Validate() error {
	if !c.Name.Valid() || c.Version == "" {
		return fmt.Errorf("unknown or missing capability name/version")
	}
	seen := make(map[PermissionScope]struct{}, len(c.Scopes))
	for _, scope := range c.Scopes {
		if !scope.Valid() {
			return fmt.Errorf("unknown permission scope %q", scope)
		}
		if _, ok := seen[scope]; ok {
			return fmt.Errorf("duplicate permission scope %q", scope)
		}
		seen[scope] = struct{}{}
	}
	return nil
}

type TenantStatus string

const (
	TenantActive   TenantStatus = "active"
	TenantDisabled TenantStatus = "disabled"
)

func (s TenantStatus) Valid() bool { return s == TenantActive || s == TenantDisabled }

type Tenant struct {
	ID       TenantID
	Name     string
	Currency Currency
	Status   TenantStatus
	Version  uint64
}

func (t Tenant) Validate() error {
	if t.ID.IsZero() || t.Name == "" || t.Version == 0 || !t.Status.Valid() {
		return fmt.Errorf("invalid tenant")
	}
	if _, err := ParseCurrency(string(t.Currency)); err != nil {
		return err
	}
	return nil
}

type Project struct {
	ID      ProjectID
	Tenant  TenantID
	Name    string
	Version uint64
}

func (p Project) Validate() error {
	if p.ID.IsZero() || p.Tenant.IsZero() || p.Name == "" || p.Version == 0 {
		return fmt.Errorf("invalid project")
	}
	return nil
}

type Principal struct {
	ID       PrincipalID
	Tenant   TenantID
	Project  ProjectID
	Scopes   []PermissionScope
	Version  uint64
	Disabled bool
}

func (p Principal) Validate() error {
	if p.ID.IsZero() || p.Tenant.IsZero() || p.Project.IsZero() || p.Version == 0 {
		return fmt.Errorf("invalid principal")
	}
	seen := make(map[PermissionScope]struct{}, len(p.Scopes))
	for _, scope := range p.Scopes {
		if !scope.Valid() {
			return fmt.Errorf("unknown permission scope %q", scope)
		}
		if _, ok := seen[scope]; ok {
			return fmt.Errorf("duplicate permission scope %q", scope)
		}
		seen[scope] = struct{}{}
	}
	return nil
}

type Request struct {
	ID        RequestID
	Tenant    TenantID
	Project   ProjectID
	Principal PrincipalID
	State     RequestState
	Model     string
	Protocol  string
}

func (r Request) Validate() error {
	if r.ID.IsZero() || r.Tenant.IsZero() || r.Project.IsZero() || r.Principal.IsZero() || r.Model == "" || r.Protocol == "" || !r.State.Valid() {
		return fmt.Errorf("invalid request")
	}
	return nil
}

type Attempt struct {
	ID                AttemptID
	Request           RequestID
	Number            uint32
	State             AttemptState
	CredentialVersion CredentialVersion
}

func (a Attempt) Validate() error {
	if a.ID.IsZero() || a.Request.IsZero() || a.Number == 0 || !a.State.Valid() || a.CredentialVersion == 0 {
		return fmt.Errorf("invalid attempt")
	}
	return nil
}

type UsageCompleteness string

const (
	UsageComplete UsageCompleteness = "complete"
	UsagePartial  UsageCompleteness = "partial"
	UsageMissing  UsageCompleteness = "missing"
)

type UsageSource string

const (
	UsageSourceUnknown  UsageSource = "unknown"
	UsageSourceProvider UsageSource = "provider"
	UsageSourceGateway  UsageSource = "gateway"
)

type UsageDimension struct {
	Value int64
	Known bool
}

type Usage struct {
	Request          RequestID
	Attempt          AttemptID
	InputTokens      int64
	OutputTokens     int64
	TotalTokens      int64
	InputKnown       bool
	OutputKnown      bool
	TotalKnown       bool
	CacheReadTokens  UsageDimension
	CacheWriteTokens UsageDimension
	ReasoningTokens  UsageDimension
	Completeness     UsageCompleteness
	Source           UsageSource
	IsEstimate       bool
	ProviderVersion  string
	RateTier         string
}

func (u Usage) Validate() error {
	if u.Request.IsZero() || u.Attempt.IsZero() || u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 || u.InputTokens > math.MaxInt64-u.OutputTokens {
		return fmt.Errorf("invalid usage")
	}
	if (!u.InputKnown && u.InputTokens != 0) || (!u.OutputKnown && u.OutputTokens != 0) || (!u.TotalKnown && u.TotalTokens != 0) {
		return fmt.Errorf("unknown usage dimensions must not carry values")
	}
	knownCount := 0
	if u.InputKnown {
		knownCount++
	}
	if u.OutputKnown {
		knownCount++
	}
	if u.TotalKnown {
		knownCount++
		if u.TotalTokens < u.InputTokens+u.OutputTokens && u.InputKnown && u.OutputKnown {
			return fmt.Errorf("total usage is smaller than known input and output")
		}
		if u.InputKnown && u.TotalTokens < u.InputTokens || u.OutputKnown && u.TotalTokens < u.OutputTokens {
			return fmt.Errorf("total usage is smaller than a known dimension")
		}
	}
	for _, dimension := range []UsageDimension{u.CacheReadTokens, u.CacheWriteTokens, u.ReasoningTokens} {
		if dimension.Value < 0 || (!dimension.Known && dimension.Value != 0) {
			return fmt.Errorf("invalid usage dimension")
		}
	}
	switch u.Completeness {
	case UsageComplete:
		if knownCount != 3 || u.Source == UsageSourceUnknown {
			return fmt.Errorf("complete usage must provide all primary dimensions and a source")
		}
	case UsagePartial:
		if knownCount == 0 || knownCount == 3 {
			return fmt.Errorf("partial usage must have both known and missing primary dimensions")
		}
	case UsageMissing:
		if knownCount != 0 {
			return fmt.Errorf("missing usage cannot mark a primary dimension known")
		}
	default:
		return fmt.Errorf("invalid usage completeness %q", u.Completeness)
	}
	switch u.Source {
	case UsageSourceUnknown, UsageSourceProvider, UsageSourceGateway:
	default:
		return fmt.Errorf("invalid usage source %q", u.Source)
	}
	return nil
}

type CredentialVersion uint64

func NewCredentialVersion(value uint64) (CredentialVersion, error) {
	if value == 0 {
		return 0, fmt.Errorf("credential version must be positive")
	}
	return CredentialVersion(value), nil
}
