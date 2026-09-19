package domain

import "fmt"

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

type CapabilityDescriptor struct {
	Name    CapabilityName
	Version string
	Enabled bool
	Scopes  []PermissionScope
}

func (c CapabilityDescriptor) Validate() error {
	if c.Name == "" || c.Version == "" {
		return fmt.Errorf("capability name and version are required")
	}
	for _, scope := range c.Scopes {
		if !scope.Valid() {
			return fmt.Errorf("unknown permission scope %q", scope)
		}
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

type Project struct {
	ID      ProjectID
	Tenant  TenantID
	Name    string
	Version uint64
}

type Principal struct {
	ID       PrincipalID
	Tenant   TenantID
	Project  ProjectID
	Scopes   []PermissionScope
	Version  uint64
	Disabled bool
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

type Attempt struct {
	ID                AttemptID
	Request           RequestID
	Number            uint32
	State             AttemptState
	CredentialVersion uint64
}

type UsageCompleteness string

const (
	UsageComplete UsageCompleteness = "complete"
	UsagePartial  UsageCompleteness = "partial"
	UsageMissing  UsageCompleteness = "missing"
)

type Usage struct {
	Request      RequestID
	Attempt      AttemptID
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	Completeness UsageCompleteness
}

type CredentialVersion uint64

func NewCredentialVersion(value uint64) (CredentialVersion, error) {
	if value == 0 {
		return 0, fmt.Errorf("credential version must be positive")
	}
	return CredentialVersion(value), nil
}
