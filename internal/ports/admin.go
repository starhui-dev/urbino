package ports

import (
	"context"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/domain"
)

// AdminMutationRequest identifies one tenant-scoped management mutation.
type AdminMutationRequest struct {
	Principal     domain.PrincipalID
	Operation     string
	KeyDigest     []byte
	RequestDigest []byte
}

// AdminMutationResponse is a safe JSON response persisted for idempotent replay.
type AdminMutationResponse struct {
	Status      int
	ContentType string
	Body        []byte
}

// AdminMutationOutcome carries the first response and the non-secret response
// that may be persisted for later replay.
type AdminMutationOutcome struct {
	Response       AdminMutationResponse
	ReplayResponse AdminMutationResponse
}

// AdminMutationResult is returned after a committed mutation or a replay.
type AdminMutationResult struct {
	Response AdminMutationResponse
	Replayed bool
}

// AdminMutationTx exposes only transaction-bound management operations.
type AdminMutationTx interface {
	CreateTenantFor(context.Context, domain.PrincipalID, domain.Tenant) error
	UpdateTenantFor(context.Context, domain.PrincipalID, domain.TenantID, uint64, string, domain.TenantStatus) (domain.Tenant, error)
	CreateAPIKeyFor(context.Context, domain.PrincipalID, auth.IssuedKey) error
	RevokeAPIKeyFor(context.Context, domain.PrincipalID, domain.TenantID, domain.ProjectID, string, time.Time) (auth.KeyRecord, error)
	RevokeAdminTokenFor(context.Context, domain.PrincipalID, domain.PrincipalID, string, time.Time) (auth.AdminTokenRecord, error)
}

// AdminMutationExecutor atomically claims, mutates, audits and stores a safe
// replay response for one management request.
type AdminMutationExecutor interface {
	RunAdminMutation(context.Context, AdminMutationRequest, func(AdminMutationTx) (AdminMutationOutcome, error)) (AdminMutationResult, error)
}
