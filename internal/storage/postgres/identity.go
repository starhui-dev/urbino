package postgres

import (
	"context"
	"errors"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// IdentityStore contains only scoped identity operations. Every mutation and
// lookup requires the tenant/project scope supplied by the caller.
type IdentityStore struct {
	db *DB
}

func NewIdentityStore(db *DB) (*IdentityStore, error) {
	if db == nil || db.Pool() == nil {
		return nil, errors.New("identity database is not open")
	}
	return &IdentityStore{db: db}, nil
}

func (s *IdentityStore) CreateTenant(ctx context.Context, tenant domain.Tenant) error {
	if err := tenant.Validate(); err != nil {
		return err
	}
	_, err := s.db.Pool().Exec(ctx, `
		INSERT INTO tenants (id, name, status, currency, policy_version)
		VALUES ($1, $2, $3, $4, $5)`, uuidArg(tenant.ID), tenant.Name, string(tenant.Status), string(tenant.Currency), tenant.Version)
	if err != nil {
		return classifyError(err)
	}
	return nil
}

func (s *IdentityStore) CreateUser(ctx context.Context, tenant domain.TenantID, user domain.PrincipalID, displayName string) error {
	if tenant.IsZero() || user.IsZero() || displayName == "" {
		return errors.New("invalid scoped user")
	}
	_, err := s.db.Pool().Exec(ctx, `
		INSERT INTO users (tenant_id, id, display_name, status)
		VALUES ($1, $2, $3, 'active')`, uuidArg(tenant), uuidArg(user), displayName)
	if err != nil {
		return classifyError(err)
	}
	return nil
}

func (s *IdentityStore) AddProjectMember(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, user domain.PrincipalID, role string) error {
	if tenant.IsZero() || project.IsZero() || user.IsZero() || role == "" {
		return errors.New("invalid project membership")
	}
	_, err := s.db.Pool().Exec(ctx, `
		INSERT INTO project_members (tenant_id, project_id, user_id, role, status)
		VALUES ($1, $2, $3, $4, 'active')`, uuidArg(tenant), uuidArg(project), uuidArg(user), role)
	if err != nil {
		return classifyError(err)
	}
	return nil
}

func (s *IdentityStore) IsActiveMember(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, user domain.PrincipalID) (bool, error) {
	if tenant.IsZero() || project.IsZero() || user.IsZero() {
		return false, errors.New("invalid project membership scope")
	}
	var active bool
	err := s.db.Pool().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM project_members
			WHERE tenant_id = $1 AND project_id = $2 AND user_id = $3 AND status = 'active'
		)`, uuidArg(tenant), uuidArg(project), uuidArg(user)).Scan(&active)
	if err != nil {
		return false, classifyError(err)
	}
	return active, nil
}

func (s *IdentityStore) CreateAPIKey(ctx context.Context, issued auth.IssuedKey) error {
	r := issued.Record
	if issued.Secret == "" || issued.Value == "" || r.ID.IsZero() || r.Tenant.IsZero() || r.Project.IsZero() || r.User.IsZero() || len(r.Digest) != 32 {
		return errors.New("invalid api key record")
	}
	return s.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var member bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM project_members WHERE tenant_id = $1 AND project_id = $2 AND user_id = $3 AND status = 'active')`, uuidArg(r.Tenant), uuidArg(r.Project), uuidArg(r.User)).Scan(&member); err != nil {
			return classifyError(err)
		}
		if !member {
			return errors.New("project membership required")
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO api_keys
			(tenant_id, id, project_id, user_id, public_id, secret_digest, digest_key_id, scopes, allowed_models, status, expires_at, auth_version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', $10, $11)`,
			uuidArg(r.Tenant), uuidArg(r.ID), uuidArg(r.Project), uuidArg(r.User), r.PublicID, r.Digest, r.PepperID,
			r.Scopes, r.Models, r.ExpiresAt, r.AuthVersion)
		if err != nil {
			return classifyError(err)
		}
		return nil
	})
}

func (s *IdentityStore) LookupAPIKey(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, publicID string) (auth.KeyRecord, error) {
	if tenant.IsZero() || project.IsZero() || publicID == "" {
		return auth.KeyRecord{}, errors.New("invalid scoped api key id")
	}
	var (
		id, tenantID, projectID, userID pgtype.UUID
		digest                          []byte
		pepperID, status                string
		scopes, models                  []string
		expiresAt                       time.Time
		revokedAt                       *time.Time
		authVersion                     uint64
	)
	tenantArg, projectArg := uuidArg(tenant), uuidArg(project)
	err := s.db.Pool().QueryRow(ctx, `
		SELECT id, tenant_id, project_id, user_id, secret_digest, digest_key_id, scopes, allowed_models, status, expires_at, revoked_at, auth_version
		FROM api_keys WHERE tenant_id = $1 AND project_id = $2 AND public_id = $3`,
		tenantArg, projectArg, publicID).Scan(
		&id, &tenantID, &projectID, &userID, &digest, &pepperID, &scopes, &models, &status, &expiresAt, &revokedAt, &authVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.KeyRecord{}, errors.New("api key not found")
	}
	if err != nil {
		return auth.KeyRecord{}, classifyError(err)
	}
	if status != "active" {
		return auth.KeyRecord{}, auth.ErrRevoked
	}
	return auth.KeyRecord{
		ID: domain.UUID(id.Bytes), PublicID: publicID,
		Tenant: domain.TenantID(domain.UUID(tenantID.Bytes)), Project: domain.ProjectID(domain.UUID(projectID.Bytes)),
		User: domain.PrincipalID(domain.UUID(userID.Bytes)), Digest: append([]byte(nil), digest...), PepperID: pepperID,
		Scopes: append([]string(nil), scopes...), Models: append([]string(nil), models...), Status: status, ExpiresAt: expiresAt, RevokedAt: revokedAt, AuthVersion: authVersion,
	}, nil
}

func (s *IdentityStore) RevokeAPIKey(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, publicID string, now time.Time) error {
	if tenant.IsZero() || project.IsZero() || publicID == "" {
		return errors.New("invalid api key revocation scope")
	}
	result, err := s.db.Pool().Exec(ctx, `
		UPDATE api_keys SET status = 'revoked', revoked_at = $1, auth_version = auth_version + 1
		WHERE tenant_id = $2 AND project_id = $3 AND public_id = $4 AND status = 'active'`, now, uuidArg(tenant), uuidArg(project), publicID)
	if err != nil {
		return classifyError(err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("api key not found")
	}
	return nil
}

func uuidArg(id interface {
	IsZero() bool
	String() string
}) pgtype.UUID {
	parsed, err := domain.ParseUUID(id.String())
	if err != nil || id.IsZero() {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

func scopeStrings(scopes []domain.PermissionScope) []string {
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		result = append(result, string(scope))
	}
	return result
}

func parseScopes(scopes []string) []domain.PermissionScope {
	result := make([]domain.PermissionScope, 0, len(scopes))
	for _, scope := range scopes {
		result = append(result, domain.PermissionScope(scope))
	}
	return result
}
