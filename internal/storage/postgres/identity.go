package postgres

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/domain"
	"example.com/urbino/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrNotFound = errors.New("identity record not found")

// ErrVersionConflict reports a stale optimistic-concurrency precondition on a
// scoped management mutation: the stored version differs from the expected
// one, so nothing was written.
var ErrVersionConflict = errors.New("identity record version conflict")

// ErrMembershipRequired reports a key binding whose user is not an active
// member of the explicit tenant/project scope.
var ErrMembershipRequired = errors.New("project membership required")

// ErrLastAdminToken prevents an authenticated administrator from locking the
// management plane by revoking the final currently usable token.
var ErrLastAdminToken = errors.New("cannot revoke the last available administrator token")

var ErrIdempotencyConflict = errors.New("management idempotency key already used")
var ErrIdempotencyPayloadConflict = errors.New("management idempotency payload conflict")

// auditEvent is one append-only management audit record. Every field is
// non-secret by construction: the store passes identifiers and whitelisted
// metadata only, never credential material or raw requests.
type auditEvent struct {
	tenant     domain.TenantID
	actor      domain.PrincipalID
	action     string
	targetType string
	targetID   domain.UUID
	result     string
	metadata   map[string]string
}

const (
	auditActorAdmin  = "admin"
	auditActorSystem = "system"

	auditResultSuccess = "success"

	auditTargetTenant         = "tenant"
	auditTargetAPIKey         = "api_key"
	auditTargetAdminToken     = "admin_token"
	auditTargetAdminPrincipal = "admin_principal"

	auditActionTenantCreate     = "tenant.create"
	auditActionTenantUpdate     = "tenant.update"
	auditActionAPIKeyCreate     = "api_key.create"
	auditActionAPIKeyRevoke     = "api_key.revoke"
	auditActionAdminBootstrap   = "admin.bootstrap"
	auditActionAdminTokenRevoke = "admin_token.revoke"
)

func (e auditEvent) validate() error {
	if len(e.action) == 0 || len(e.action) > 120 {
		return errors.New("audit action is invalid")
	}
	if len(e.targetType) == 0 || len(e.targetType) > 120 {
		return errors.New("audit target type is invalid")
	}
	switch e.result {
	case auditResultSuccess, "denied", "failure":
	default:
		return errors.New("audit result is invalid")
	}
	for key, value := range e.metadata {
		if key == "" || len(value) > 200 {
			return errors.New("audit metadata is invalid")
		}
	}
	return nil
}

// insertAuditEvent appends one audit row inside the caller's transaction, so
// the state change and its audit record commit or roll back together. A zero
// tenant is stored as NULL; a zero actor is recorded as the installation
// itself and a non-zero actor as an authenticated administrator.
func insertAuditEvent(ctx context.Context, tx pgx.Tx, event auditEvent) error {
	if err := event.validate(); err != nil {
		return err
	}
	actorKind := auditActorSystem
	var actorID pgtype.UUID
	if !event.actor.IsZero() {
		actorKind = auditActorAdmin
		actorID = uuidArg(event.actor)
	}
	var metadata []byte
	if len(event.metadata) == 0 {
		metadata = []byte("{}")
	} else {
		encoded, err := json.Marshal(event.metadata)
		if err != nil {
			return errors.New("audit metadata is invalid")
		}
		metadata = encoded
	}
	eventID, err := domain.NewUUID()
	if err != nil {
		return errors.New("audit identity generation failed")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (id, tenant_id, actor_kind, actor_id, action, target_type, target_id, reason, result, safe_metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULL, $8, $9::jsonb)`,
		uuidArg(eventID), uuidArg(event.tenant), actorKind, actorID, event.action, event.targetType,
		uuidArg(event.targetID), event.result, metadata); err != nil {
		return classifyError(err)
	}
	return nil
}
func activeProjectMember(ctx context.Context, tx pgx.Tx, tenant domain.TenantID, project domain.ProjectID, user domain.PrincipalID) (bool, error) {
	var member bool
	if err := tx.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1
            FROM project_members pm
            JOIN users u ON u.tenant_id = pm.tenant_id AND u.id = pm.user_id
            WHERE pm.tenant_id = $1 AND pm.project_id = $2 AND pm.user_id = $3
              AND pm.status = 'active' AND u.status = 'active'
        )`, uuidArg(tenant), uuidArg(project), uuidArg(user)).Scan(&member); err != nil {
		return false, classifyError(err)
	}
	return member, nil
}

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

// CreateTenant is the system entrypoint used by fixture and installation
// tooling; authenticated management callers use CreateTenantFor.
func (s *IdentityStore) CreateTenant(ctx context.Context, tenant domain.Tenant) error {
	return s.CreateTenantFor(ctx, domain.PrincipalID{}, tenant)
}

// CreateTenantFor creates one tenant and its audit event in one transaction.
func (s *IdentityStore) CreateTenantFor(ctx context.Context, actor domain.PrincipalID, tenant domain.Tenant) error {
	if err := tenant.Validate(); err != nil {
		return err
	}
	return s.db.WithTxProbe(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
            INSERT INTO tenants (id, name, status, currency, policy_version)
            VALUES ($1, $2, $3, $4, $5)`, uuidArg(tenant.ID), tenant.Name, string(tenant.Status), string(tenant.Currency), tenant.Version); err != nil {
			return classifyError(err)
		}
		return insertAuditEvent(ctx, tx, auditEvent{
			tenant: tenant.ID, actor: actor, action: auditActionTenantCreate,
			targetType: auditTargetTenant, targetID: domain.UUID(tenant.ID), result: auditResultSuccess,
			metadata: map[string]string{"status": string(tenant.Status), "currency": string(tenant.Currency)},
		})
	}, func(ctx context.Context) (bool, error) {
		var exists bool
		err := s.db.Pool().QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenants WHERE id = $1)`, uuidArg(tenant.ID)).Scan(&exists)
		return exists, err
	})
}

// GetTenant reads one tenant by identifier.
func (s *IdentityStore) GetTenant(ctx context.Context, id domain.TenantID) (domain.Tenant, error) {
	if id.IsZero() {
		return domain.Tenant{}, errors.New("invalid tenant identifier")
	}
	var (
		tenantID         pgtype.UUID
		status, currency string
		tenant           domain.Tenant
	)
	err := s.db.Pool().QueryRow(ctx, `
		SELECT id, name, status, currency, policy_version FROM tenants WHERE id = $1`, uuidArg(id)).Scan(
		&tenantID, &tenant.Name, &status, &currency, &tenant.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Tenant{}, ErrNotFound
	}
	if err != nil {
		return domain.Tenant{}, classifyError(err)
	}
	tenant.ID = domain.TenantID(domain.UUID(tenantID.Bytes))
	tenant.Status = domain.TenantStatus(status)
	tenant.Currency = domain.Currency(currency)
	return tenant, nil
}

type tenantCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func encodeTenantCursor(createdAt time.Time, id domain.TenantID) (string, error) {
	raw, err := json.Marshal(tenantCursor{CreatedAt: createdAt.UTC(), ID: id.String()})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeTenantCursor(raw string) (tenantCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return tenantCursor{}, errors.New("invalid tenant cursor")
	}
	var cursor tenantCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.CreatedAt.IsZero() {
		return tenantCursor{}, errors.New("invalid tenant cursor")
	}
	if _, err := domain.ParseTenantID(cursor.ID); err != nil {
		return tenantCursor{}, errors.New("invalid tenant cursor")
	}
	return cursor, nil
}

// ListTenants keeps the legacy first-page helper for fixtures.
func (s *IdentityStore) ListTenants(ctx context.Context, limit int) ([]domain.Tenant, error) {
	tenants, _, err := s.ListTenantsPage(ctx, limit, "")
	return tenants, err
}

// ListTenantsPage returns a stable creation-order page and an opaque cursor.
func (s *IdentityStore) ListTenantsPage(ctx context.Context, limit int, rawCursor string) ([]domain.Tenant, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var cursor tenantCursor
	if rawCursor != "" {
		var err error
		cursor, err = decodeTenantCursor(rawCursor)
		if err != nil {
			return nil, "", err
		}
	}
	query := `SELECT id, name, status, currency, policy_version, created_at FROM tenants ORDER BY created_at, id LIMIT $1`
	args := []any{limit + 1}
	if rawCursor != "" {
		parsedID, err := domain.ParseTenantID(cursor.ID)
		if err != nil {
			return nil, "", errors.New("invalid tenant cursor")
		}
		query = `SELECT id, name, status, currency, policy_version, created_at FROM tenants WHERE (created_at, id) > ($1, $2) ORDER BY created_at, id LIMIT $3`
		args = []any{cursor.CreatedAt, uuidArg(parsedID), limit + 1}
	}
	rows, err := s.db.Pool().Query(ctx, query, args...)
	if err != nil {
		return nil, "", classifyError(err)
	}
	defer rows.Close()
	result := make([]domain.Tenant, 0, limit+1)
	var lastPageCreatedAt time.Time
	for rows.Next() {
		var tenantID pgtype.UUID
		var status, currency string
		var createdAt time.Time
		var tenant domain.Tenant
		if err := rows.Scan(&tenantID, &tenant.Name, &status, &currency, &tenant.Version, &createdAt); err != nil {
			return nil, "", classifyError(err)
		}
		tenant.ID = domain.TenantID(domain.UUID(tenantID.Bytes))
		tenant.Status = domain.TenantStatus(status)
		tenant.Currency = domain.Currency(currency)
		result = append(result, tenant)
		if len(result) <= limit {
			lastPageCreatedAt = createdAt
		}
		if len(result) == limit+1 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", classifyError(err)
	}
	if len(result) <= limit {
		return result, "", nil
	}
	last := result[limit-1]
	next, err := encodeTenantCursor(lastPageCreatedAt, last.ID)
	if err != nil {
		return nil, "", classifyError(err)
	}
	return result[:limit], next, nil
}

// UpdateTenantFor updates tenant name/status under an optimistic concurrency
// check: expectedVersion must equal the stored policy_version. The new tenant
// state and its audit event are written in one transaction, so a stale writer
// leaves no audit trail for a change that did not happen. A stale version
// returns ErrVersionConflict and an unknown tenant returns ErrNotFound.
func (s *IdentityStore) UpdateTenantFor(ctx context.Context, actor domain.PrincipalID, id domain.TenantID, expectedVersion uint64, name string, status domain.TenantStatus) (domain.Tenant, error) {
	if id.IsZero() || expectedVersion == 0 || len(name) == 0 || len(name) > 200 || !status.Valid() {
		return domain.Tenant{}, errors.New("invalid tenant update")
	}
	var updated domain.Tenant
	err := s.db.WithTxProbe(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var (
			tenantID         pgtype.UUID
			stored, currency string
			version          uint64
		)
		err := tx.QueryRow(ctx, `
            UPDATE tenants SET name = $1, status = $2, policy_version = policy_version + 1
            WHERE id = $3 AND policy_version = $4
            RETURNING id, name, status, currency, policy_version`,
			name, string(status), uuidArg(id), expectedVersion).Scan(
			&tenantID, &updated.Name, &stored, &currency, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenants WHERE id = $1)`, uuidArg(id)).Scan(&exists); err != nil {
				return classifyError(err)
			}
			if !exists {
				return ErrNotFound
			}
			return ErrVersionConflict
		}
		if err != nil {
			return classifyError(err)
		}
		updated.ID = domain.TenantID(domain.UUID(tenantID.Bytes))
		updated.Status = domain.TenantStatus(stored)
		updated.Currency = domain.Currency(currency)
		updated.Version = version
		return insertAuditEvent(ctx, tx, auditEvent{
			tenant: updated.ID, actor: actor, action: auditActionTenantUpdate,
			targetType: auditTargetTenant, targetID: domain.UUID(updated.ID), result: auditResultSuccess,
			metadata: map[string]string{"status": string(updated.Status), "version": strconv.FormatUint(version, 10)},
		})
	}, func(ctx context.Context) (bool, error) {
		var exists bool
		err := s.db.Pool().QueryRow(ctx, `
            SELECT EXISTS (SELECT 1 FROM tenants WHERE id = $1 AND name = $2 AND status = $3 AND policy_version = $4)`,
			uuidArg(id), name, string(status), expectedVersion+1).Scan(&exists)
		return exists, err
	})
	if err != nil {
		return domain.Tenant{}, err
	}
	return updated, nil
}
func (s *IdentityStore) CreateProject(ctx context.Context, project domain.Project) error {
	if err := project.Validate(); err != nil {
		return err
	}
	return s.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
            INSERT INTO projects (tenant_id, id, name, status)
            VALUES ($1, $2, $3, 'active')`, uuidArg(project.Tenant), uuidArg(project.ID), project.Name); err != nil {
			return classifyError(err)
		}
		return insertAuditEvent(ctx, tx, auditEvent{
			tenant: project.Tenant, action: "project.create", targetType: "project", targetID: domain.UUID(project.ID),
			result: auditResultSuccess, metadata: map[string]string{"status": "active"},
		})
	})
}

// EnsureUser is reserved for deterministic integration fixtures. It is
// idempotent only for the same tenant-scoped identity.
func (s *IdentityStore) EnsureUser(ctx context.Context, tenant domain.TenantID, user domain.PrincipalID, displayName string) error {
	if tenant.IsZero() || user.IsZero() || displayName == "" {
		return errors.New("invalid scoped user")
	}
	return s.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
            INSERT INTO users (tenant_id, id, display_name, status)
            VALUES ($1, $2, $3, 'active')
            ON CONFLICT (tenant_id, id) DO NOTHING`, uuidArg(tenant), uuidArg(user), displayName); err != nil {
			return classifyError(err)
		}
		return insertAuditEvent(ctx, tx, auditEvent{
			tenant: tenant, action: "user.ensure", targetType: "user", targetID: domain.UUID(user),
			result: auditResultSuccess, metadata: map[string]string{"status": "active"},
		})
	})
}

// EnsureProjectMember is reserved for deterministic integration fixtures. It
// never grants membership outside the explicit tenant/project scope.
func (s *IdentityStore) EnsureProjectMember(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, user domain.PrincipalID, role string) error {
	if tenant.IsZero() || project.IsZero() || user.IsZero() || role == "" {
		return errors.New("invalid project membership")
	}
	return s.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
            INSERT INTO project_members (tenant_id, project_id, user_id, role, status)
            VALUES ($1, $2, $3, $4, 'active')
            ON CONFLICT (tenant_id, project_id, user_id)
            DO UPDATE SET role = EXCLUDED.role, status = 'active'`, uuidArg(tenant), uuidArg(project), uuidArg(user), role); err != nil {
			return classifyError(err)
		}
		return insertAuditEvent(ctx, tx, auditEvent{
			tenant: tenant, action: "project_member.ensure", targetType: "project_member", targetID: domain.UUID(user),
			result: auditResultSuccess, metadata: map[string]string{"project_id": project.String(), "role": role},
		})
	})
}

func (s *IdentityStore) IsActiveMember(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, user domain.PrincipalID) (bool, error) {
	if tenant.IsZero() || project.IsZero() || user.IsZero() {
		return false, errors.New("invalid project membership scope")
	}
	var active bool
	err := s.db.Pool().QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1 FROM project_members pm
            JOIN users u ON u.tenant_id = pm.tenant_id AND u.id = pm.user_id
            WHERE pm.tenant_id = $1 AND pm.project_id = $2 AND pm.user_id = $3
              AND pm.status = 'active' AND u.status = 'active'
        )`, uuidArg(tenant), uuidArg(project), uuidArg(user)).Scan(&active)
	if err != nil {
		return false, classifyError(err)
	}
	return active, nil
}

// CreateAPIKey is the system entrypoint used by fixtures and installation
// tooling; authenticated management callers use CreateAPIKeyFor.
func (s *IdentityStore) CreateAPIKey(ctx context.Context, issued auth.IssuedKey) error {
	return s.CreateAPIKeyFor(ctx, domain.PrincipalID{}, issued)
}

// CreateAPIKeyFor persists a newly issued public key and its audit event in
// one transaction: a key without an audit record, or an audit record without
// its key, cannot become visible. The key is only accepted for an active
// member of the explicit tenant/project scope, and the plaintext secret is
// never stored.
func (s *IdentityStore) CreateAPIKeyFor(ctx context.Context, actor domain.PrincipalID, issued auth.IssuedKey) error {
	r := issued.Record
	if err := validatePublicKeyScopes(r.Scopes); err != nil {
		return err
	}
	if issued.Secret == "" || issued.Value == "" || r.ID.IsZero() || r.Tenant.IsZero() || r.Project.IsZero() || r.User.IsZero() || len(r.Digest) != 32 {
		return errors.New("invalid api key record")
	}
	return s.db.WithTxProbe(ctx, func(ctx context.Context, tx pgx.Tx) error {
		member, err := activeProjectMember(ctx, tx, r.Tenant, r.Project, r.User)
		if err != nil {
			return err
		}
		if !member {
			return ErrMembershipRequired
		}
		if _, err := tx.Exec(ctx, `
            INSERT INTO api_keys
            (tenant_id, id, project_id, user_id, public_id, secret_digest, digest_key_id, scopes, allowed_models, status, expires_at, auth_version)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', $10, $11)`,
			uuidArg(r.Tenant), uuidArg(r.ID), uuidArg(r.Project), uuidArg(r.User), r.PublicID, r.Digest, r.PepperID,
			r.Scopes, r.Models, r.ExpiresAt, r.AuthVersion); err != nil {
			return classifyError(err)
		}
		return insertAuditEvent(ctx, tx, auditEvent{
			tenant: r.Tenant, actor: actor, action: auditActionAPIKeyCreate,
			targetType: auditTargetAPIKey, targetID: r.ID, result: auditResultSuccess,
			metadata: map[string]string{"status": "active", "auth_version": strconv.FormatUint(r.AuthVersion, 10)},
		})
	}, func(ctx context.Context) (bool, error) {
		var exists bool
		err := s.db.Pool().QueryRow(ctx, `
            SELECT EXISTS (SELECT 1 FROM api_keys WHERE id = $1 AND tenant_id = $2 AND public_id = $3)`,
			uuidArg(r.ID), uuidArg(r.Tenant), r.PublicID).Scan(&exists)
		return exists, err
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
        SELECT k.id, k.tenant_id, k.project_id, k.user_id, k.secret_digest, k.digest_key_id,
               k.scopes, k.allowed_models, k.status, k.expires_at, k.revoked_at, k.auth_version
        FROM api_keys k
        JOIN tenants t ON t.id = k.tenant_id AND t.status = 'active'
        JOIN projects p ON p.tenant_id = k.tenant_id AND p.id = k.project_id AND p.status = 'active'
        JOIN users u ON u.tenant_id = k.tenant_id AND u.id = k.user_id AND u.status = 'active'
        JOIN project_members pm ON pm.tenant_id = k.tenant_id AND pm.project_id = k.project_id AND pm.user_id = k.user_id AND pm.status = 'active'
        WHERE k.tenant_id = $1 AND k.project_id = $2 AND k.public_id = $3`,
		tenantArg, projectArg, publicID).Scan(
		&id, &tenantID, &projectID, &userID, &digest, &pepperID, &scopes, &models, &status, &expiresAt, &revokedAt, &authVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.KeyRecord{}, ErrNotFound
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
func (s *IdentityStore) ListAPIKeys(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, status string, limit int) ([]auth.KeyRecord, error) {
	return s.ListAPIKeysAt(ctx, tenant, project, status, limit, time.Now().UTC())
}

func (s *IdentityStore) ListAPIKeysAt(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, status string, limit int, now time.Time) ([]auth.KeyRecord, error) {
	if tenant.IsZero() || project.IsZero() {
		return nil, errors.New("invalid scoped api key list")
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.Pool().Query(ctx, `
        SELECT id, tenant_id, project_id, user_id, public_id, secret_digest, digest_key_id,
               scopes, allowed_models,
               CASE WHEN status = 'active' AND expires_at <= $3 THEN 'expired' ELSE status END,
               expires_at, revoked_at, auth_version
        FROM api_keys
        WHERE tenant_id = $1 AND project_id = $2
          AND ($4 = '' OR CASE WHEN status = 'active' AND expires_at <= $3 THEN 'expired' ELSE status END = $4)
        ORDER BY created_at, public_id
        LIMIT $5`, uuidArg(tenant), uuidArg(project), now, status, limit)
	if err != nil {
		return nil, classifyError(err)
	}
	defer rows.Close()
	result := make([]auth.KeyRecord, 0)
	for rows.Next() {
		record, err := scanKeyRecord(rows)
		if err != nil {
			return nil, classifyError(err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyError(err)
	}
	return result, nil
}

func (s *IdentityStore) CountAPIKeys(ctx context.Context, tenant domain.TenantID, project domain.ProjectID) (total, active, revoked int, err error) {
	return s.CountAPIKeysAt(ctx, tenant, project, time.Now().UTC())
}

func (s *IdentityStore) CountAPIKeysAt(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, now time.Time) (total, active, revoked int, err error) {
	if tenant.IsZero() || project.IsZero() {
		return 0, 0, 0, errors.New("invalid scoped api key statistics")
	}
	err = s.db.Pool().QueryRow(ctx, `
        SELECT count(*)::int,
               count(*) FILTER (WHERE status = 'active' AND expires_at > $3)::int,
               count(*) FILTER (WHERE status = 'revoked')::int
        FROM api_keys WHERE tenant_id = $1 AND project_id = $2`, uuidArg(tenant), uuidArg(project), now).Scan(
		&total, &active, &revoked)
	if err != nil {
		return 0, 0, 0, classifyError(err)
	}
	return total, active, revoked, nil
}

type adminMutationTx struct {
	tx pgx.Tx
}

func (t adminMutationTx) CreateTenantFor(ctx context.Context, actor domain.PrincipalID, tenant domain.Tenant) error {
	if err := tenant.Validate(); err != nil {
		return err
	}
	if _, err := t.tx.Exec(ctx, `
        INSERT INTO tenants (id, name, status, currency, policy_version)
        VALUES ($1, $2, $3, $4, $5)`, uuidArg(tenant.ID), tenant.Name, string(tenant.Status), string(tenant.Currency), tenant.Version); err != nil {
		return classifyError(err)
	}
	return insertAuditEvent(ctx, t.tx, auditEvent{
		tenant: tenant.ID, actor: actor, action: auditActionTenantCreate,
		targetType: auditTargetTenant, targetID: domain.UUID(tenant.ID), result: auditResultSuccess,
		metadata: map[string]string{"status": string(tenant.Status), "currency": string(tenant.Currency)},
	})
}

func (t adminMutationTx) UpdateTenantFor(ctx context.Context, actor domain.PrincipalID, id domain.TenantID, expectedVersion uint64, name string, status domain.TenantStatus) (domain.Tenant, error) {
	if id.IsZero() || expectedVersion == 0 || len(name) == 0 || len(name) > 200 || !status.Valid() {
		return domain.Tenant{}, errors.New("invalid tenant update")
	}
	var (
		tenantID         pgtype.UUID
		stored, currency string
		version          uint64
		updated          domain.Tenant
	)
	err := t.tx.QueryRow(ctx, `
        UPDATE tenants SET name = $1, status = $2, policy_version = policy_version + 1
        WHERE id = $3 AND policy_version = $4
        RETURNING id, name, status, currency, policy_version`,
		name, string(status), uuidArg(id), expectedVersion).Scan(&tenantID, &updated.Name, &stored, &currency, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if err := t.tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenants WHERE id = $1)`, uuidArg(id)).Scan(&exists); err != nil {
			return domain.Tenant{}, classifyError(err)
		}
		if !exists {
			return domain.Tenant{}, ErrNotFound
		}
		return domain.Tenant{}, ErrVersionConflict
	}
	if err != nil {
		return domain.Tenant{}, classifyError(err)
	}
	updated.ID = domain.TenantID(domain.UUID(tenantID.Bytes))
	updated.Status = domain.TenantStatus(stored)
	updated.Currency = domain.Currency(currency)
	updated.Version = version
	if err := insertAuditEvent(ctx, t.tx, auditEvent{
		tenant: updated.ID, actor: actor, action: auditActionTenantUpdate,
		targetType: auditTargetTenant, targetID: domain.UUID(updated.ID), result: auditResultSuccess,
		metadata: map[string]string{"status": string(updated.Status), "version": strconv.FormatUint(version, 10)},
	}); err != nil {
		return domain.Tenant{}, err
	}
	return updated, nil
}

func (t adminMutationTx) CreateAPIKeyFor(ctx context.Context, actor domain.PrincipalID, issued auth.IssuedKey) error {
	r := issued.Record
	if err := validatePublicKeyScopes(r.Scopes); err != nil {
		return err
	}
	if issued.Secret == "" || issued.Value == "" || r.ID.IsZero() || r.Tenant.IsZero() || r.Project.IsZero() || r.User.IsZero() || len(r.Digest) != 32 {
		return errors.New("invalid api key record")
	}
	member, err := activeProjectMember(ctx, t.tx, r.Tenant, r.Project, r.User)
	if err != nil {
		return err
	}
	if !member {
		return ErrMembershipRequired
	}
	if _, err := t.tx.Exec(ctx, `
        INSERT INTO api_keys
        (tenant_id, id, project_id, user_id, public_id, secret_digest, digest_key_id, scopes, allowed_models, status, expires_at, auth_version)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', $10, $11)`,
		uuidArg(r.Tenant), uuidArg(r.ID), uuidArg(r.Project), uuidArg(r.User), r.PublicID, r.Digest, r.PepperID,
		r.Scopes, r.Models, r.ExpiresAt, r.AuthVersion); err != nil {
		return classifyError(err)
	}
	return insertAuditEvent(ctx, t.tx, auditEvent{
		tenant: r.Tenant, actor: actor, action: auditActionAPIKeyCreate,
		targetType: auditTargetAPIKey, targetID: r.ID, result: auditResultSuccess,
		metadata: map[string]string{"status": "active", "auth_version": strconv.FormatUint(r.AuthVersion, 10)},
	})
}

func (t adminMutationTx) RevokeAPIKeyFor(ctx context.Context, actor domain.PrincipalID, tenant domain.TenantID, project domain.ProjectID, publicID string, now time.Time) (auth.KeyRecord, error) {
	if tenant.IsZero() || project.IsZero() || publicID == "" {
		return auth.KeyRecord{}, errors.New("invalid api key revocation scope")
	}
	row := t.tx.QueryRow(ctx, `
        UPDATE api_keys SET status = 'revoked', revoked_at = $1, auth_version = auth_version + 1
        WHERE tenant_id = $2 AND project_id = $3 AND public_id = $4 AND status = 'active'
        RETURNING id, tenant_id, project_id, user_id, public_id, secret_digest, digest_key_id,
                  scopes, allowed_models, status, expires_at, revoked_at, auth_version`,
		now, uuidArg(tenant), uuidArg(project), publicID)
	revoked, err := scanKeyRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.KeyRecord{}, ErrNotFound
	}
	if err != nil {
		return auth.KeyRecord{}, classifyError(err)
	}
	if err := insertAuditEvent(ctx, t.tx, auditEvent{
		tenant: revoked.Tenant, actor: actor, action: auditActionAPIKeyRevoke,
		targetType: auditTargetAPIKey, targetID: revoked.ID, result: auditResultSuccess,
		metadata: map[string]string{"status": revoked.Status, "auth_version": strconv.FormatUint(revoked.AuthVersion, 10)},
	}); err != nil {
		return auth.KeyRecord{}, err
	}
	return revoked, nil
}

func (t adminMutationTx) RevokeAdminTokenFor(ctx context.Context, actor domain.PrincipalID, principal domain.PrincipalID, publicID string, now time.Time) (auth.AdminTokenRecord, error) {
	if principal.IsZero() || publicID == "" {
		return auth.AdminTokenRecord{}, errors.New("invalid administrator token revocation scope")
	}
	if _, err := t.tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(0x5552444e41444d49)); err != nil {
		return auth.AdminTokenRecord{}, classifyError(err)
	}
	var available int
	if err := t.tx.QueryRow(ctx, `
        SELECT count(*) FROM admin_tokens t
        JOIN admin_principals p ON p.id = t.principal_id
        WHERE t.revoked_at IS NULL AND t.expires_at > $1 AND p.status = 'active'`, now).Scan(&available); err != nil {
		return auth.AdminTokenRecord{}, classifyError(err)
	}
	if available <= 1 {
		return auth.AdminTokenRecord{}, ErrLastAdminToken
	}
	var record auth.AdminTokenRecord
	if err := revokeAdminTokenTx(ctx, t.tx, actor, principal, publicID, now, &record); err != nil {
		return auth.AdminTokenRecord{}, err
	}
	return record, nil
}

type idempotencyRow struct {
	requestDigest []byte
	state         string
	status        *int
	contentType   *string
	body          []byte
	terminalized  bool
}

const legacyPendingResponseBody = `{"code":"idempotency_outcome_unknown","message":"the previous management mutation outcome is unavailable; do not retry with this key","request_id":"00000000-0000-0000-0000-000000000000","retryable":false,"details":{}}`

func (s *IdentityStore) RunAdminMutation(ctx context.Context, request ports.AdminMutationRequest, fn func(ports.AdminMutationTx) (ports.AdminMutationOutcome, error)) (result ports.AdminMutationResult, err error) {
	if request.Principal.IsZero() || request.Operation == "" || len(request.KeyDigest) != 32 || len(request.RequestDigest) != 32 || fn == nil {
		return result, errors.New("invalid management mutation request")
	}
	var outcome ports.AdminMutationOutcome
	err = s.db.WithTxProbe(ctx, func(ctx context.Context, tx pgx.Tx) error {
		inserted, existing, err := claimAdminIdempotencyTx(ctx, tx, request)
		if err != nil {
			return err
		}
		if !inserted {
			if !bytes.Equal(existing.requestDigest, request.RequestDigest) {
				if existing.terminalized {
					result = ports.AdminMutationResult{Replayed: true, Response: ports.AdminMutationResponse{
						Status: *existing.status, ContentType: *existing.contentType, Body: append([]byte(nil), existing.body...),
					}}
					return nil
				}
				return ErrIdempotencyPayloadConflict
			}
			if existing.state == "completed" {
				if existing.status == nil || existing.contentType == nil || len(existing.body) == 0 {
					return errors.New("completed idempotency record is incomplete")
				}
				result = ports.AdminMutationResult{Replayed: true, Response: ports.AdminMutationResponse{
					Status: *existing.status, ContentType: *existing.contentType, Body: append([]byte(nil), existing.body...),
				}}
				return nil
			}
			return ErrIdempotencyConflict
		}
		outcome, err = fn(adminMutationTx{tx: tx})
		if err != nil {
			return err
		}
		if err := validateAdminMutationResponse(outcome.Response); err != nil {
			return err
		}
		if err := validateAdminMutationResponse(outcome.ReplayResponse); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
            UPDATE admin_idempotency
            SET state = 'completed', response_status = $1, response_content_type = $2,
                response_body = $3, completed_at = now()
            WHERE principal_id = $4 AND operation = $5 AND key_digest = $6 AND state = 'pending'`,
			outcome.ReplayResponse.Status, outcome.ReplayResponse.ContentType, outcome.ReplayResponse.Body,
			uuidArg(request.Principal), request.Operation, request.KeyDigest)
		if err != nil {
			return classifyError(err)
		}
		return nil
	}, func(ctx context.Context) (bool, error) {
		var row idempotencyRow
		err := s.db.Pool().QueryRow(ctx, `
            SELECT request_digest, state, response_status, response_content_type, response_body
            FROM admin_idempotency
            WHERE principal_id = $1 AND operation = $2 AND key_digest = $3 AND request_digest = $4`,
			uuidArg(request.Principal), request.Operation, request.KeyDigest, request.RequestDigest).Scan(
			&row.requestDigest, &row.state, &row.status, &row.contentType, &row.body)
		if err != nil {
			return false, err
		}
		if row.state != "completed" {
			return false, nil
		}
		if row.status == nil || row.contentType == nil || len(row.body) == 0 {
			return false, errors.New("completed idempotency record is incomplete")
		}
		result = ports.AdminMutationResult{Replayed: true, Response: ports.AdminMutationResponse{
			Status: *row.status, ContentType: *row.contentType, Body: append([]byte(nil), row.body...),
		}}
		return true, nil
	})
	if err != nil {
		return ports.AdminMutationResult{}, err
	}
	if result.Replayed {
		return result, nil
	}
	result.Response = outcome.Response
	return result, nil
}

func claimAdminIdempotencyTx(ctx context.Context, tx pgx.Tx, request ports.AdminMutationRequest) (bool, idempotencyRow, error) {
	tag, err := tx.Exec(ctx, `
        INSERT INTO admin_idempotency (principal_id, operation, key_digest, request_digest)
        VALUES ($1, $2, $3, $4) ON CONFLICT (principal_id, operation, key_digest) DO NOTHING`,
		uuidArg(request.Principal), request.Operation, request.KeyDigest, request.RequestDigest)
	if err != nil {
		return false, idempotencyRow{}, classifyError(err)
	}
	if tag.RowsAffected() == 1 {
		return true, idempotencyRow{}, nil
	}
	var row idempotencyRow
	err = tx.QueryRow(ctx, `
        SELECT request_digest, state, response_status, response_content_type, response_body
        FROM admin_idempotency
        WHERE principal_id = $1 AND operation = $2 AND key_digest = $3 FOR UPDATE`,
		uuidArg(request.Principal), request.Operation, request.KeyDigest).Scan(
		&row.requestDigest, &row.state, &row.status, &row.contentType, &row.body)
	if err != nil {
		return false, idempotencyRow{}, classifyError(err)
	}
	if row.state == "pending" {
		status := http.StatusConflict
		contentType := "application/json"
		body := []byte(legacyPendingResponseBody)
		if _, err := tx.Exec(ctx, `
            UPDATE admin_idempotency
            SET state = 'completed', response_status = $1, response_content_type = $2,
                response_body = $3, completed_at = now()
            WHERE principal_id = $4 AND operation = $5 AND key_digest = $6 AND state = 'pending'`,
			status, contentType, body, uuidArg(request.Principal), request.Operation, request.KeyDigest); err != nil {
			return false, idempotencyRow{}, classifyError(err)
		}
		row.state = "completed"
		row.status = &status
		row.contentType = &contentType
		row.body = body
		row.terminalized = true
	}
	if row.state == "completed" && bytes.Equal(row.body, []byte(legacyPendingResponseBody)) {
		row.terminalized = true
	}
	return false, row, nil
}

func validateAdminMutationResponse(response ports.AdminMutationResponse) error {
	if response.Status < 200 || response.Status > 599 || response.ContentType != "application/json" || len(response.Body) == 0 || len(response.Body) > 64<<10 {
		return errors.New("invalid management idempotency response")
	}
	return nil
}

type keyRecordScanner interface {
	Scan(...any) error
}

func scanKeyRecord(row keyRecordScanner) (auth.KeyRecord, error) {
	var (
		id, tenantID, projectID, userID pgtype.UUID
		digest                          []byte
		publicID, pepperID, status      string
		scopes, models                  []string
		expiresAt                       time.Time
		revokedAt                       *time.Time
		authVersion                     uint64
	)
	if err := row.Scan(&id, &tenantID, &projectID, &userID, &publicID, &digest, &pepperID, &scopes, &models, &status, &expiresAt, &revokedAt, &authVersion); err != nil {
		return auth.KeyRecord{}, err
	}
	return auth.KeyRecord{
		ID: domain.UUID(id.Bytes), PublicID: publicID,
		Tenant: domain.TenantID(domain.UUID(tenantID.Bytes)), Project: domain.ProjectID(domain.UUID(projectID.Bytes)),
		User: domain.PrincipalID(domain.UUID(userID.Bytes)), Digest: append([]byte(nil), digest...), PepperID: pepperID,
		Scopes: append([]string(nil), scopes...), Models: append([]string(nil), models...), Status: status,
		ExpiresAt: expiresAt, RevokedAt: revokedAt, AuthVersion: authVersion,
	}, nil
}

// RevokeAPIKey is the system entrypoint used by fixtures and installation
// tooling; authenticated management callers use RevokeAPIKeyFor.
func (s *IdentityStore) RevokeAPIKey(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, publicID string, now time.Time) error {
	_, err := s.RevokeAPIKeyFor(ctx, domain.PrincipalID{}, tenant, project, publicID, now)
	return err
}

// RevokeAPIKeyFor revokes one active key inside the explicit tenant/project
// one transaction, and the returned record carries the new version so a caller
// can prove the bump. A key outside the scope, already revoked or unknown
// returns ErrNotFound.
func (s *IdentityStore) RevokeAPIKeyFor(ctx context.Context, actor domain.PrincipalID, tenant domain.TenantID, project domain.ProjectID, publicID string, now time.Time) (auth.KeyRecord, error) {
	if tenant.IsZero() || project.IsZero() || publicID == "" {
		return auth.KeyRecord{}, errors.New("invalid api key revocation scope")
	}
	var record auth.KeyRecord
	err := s.db.WithTxProbe(ctx, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
            UPDATE api_keys SET status = 'revoked', revoked_at = $1, auth_version = auth_version + 1
            WHERE tenant_id = $2 AND project_id = $3 AND public_id = $4 AND status = 'active'
            RETURNING id, tenant_id, project_id, user_id, public_id, secret_digest, digest_key_id,
                      scopes, allowed_models, status, expires_at, revoked_at, auth_version`,
			now, uuidArg(tenant), uuidArg(project), publicID)
		revoked, err := scanKeyRecord(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return classifyError(err)
		}
		if err := insertAuditEvent(ctx, tx, auditEvent{
			tenant: revoked.Tenant, actor: actor, action: auditActionAPIKeyRevoke,
			targetType: auditTargetAPIKey, targetID: revoked.ID, result: auditResultSuccess,
			metadata: map[string]string{"status": revoked.Status, "auth_version": strconv.FormatUint(revoked.AuthVersion, 10)},
		}); err != nil {
			return err
		}
		record = revoked
		return nil
	}, func(ctx context.Context) (bool, error) {
		var revoked bool
		err := s.db.Pool().QueryRow(ctx, `
            SELECT EXISTS (SELECT 1 FROM api_keys WHERE tenant_id = $1 AND project_id = $2 AND public_id = $3 AND status = 'revoked')`,
			uuidArg(tenant), uuidArg(project), publicID).Scan(&revoked)
		return revoked, err
	})
	if err != nil {
		return auth.KeyRecord{}, err
	}
	return record, nil
}

// RevokeAdminToken is the system entrypoint used by installation tooling;
// authenticated management callers use RevokeAdminTokenFor so the audit actor
// is the administrator who performed the revocation.
func (s *IdentityStore) RevokeAdminToken(ctx context.Context, principal domain.PrincipalID, publicID string, now time.Time) error {
	_, err := s.RevokeAdminTokenFor(ctx, domain.PrincipalID{}, principal, publicID, now)
	return err
}

// RevokeAdminTokenFor revokes one active administrator token inside the
// explicit principal scope: the caller can only revoke a token that belongs to
// the given principal. The revocation, the auth_version bump and the audit
// event commit in one transaction, so a leaked token cannot be revoked
// without a durable record. An unknown or already revoked token returns
// ErrNotFound.
func (s *IdentityStore) RevokeAdminTokenFor(ctx context.Context, actor domain.PrincipalID, principal domain.PrincipalID, publicID string, now time.Time) (auth.AdminTokenRecord, error) {
	if principal.IsZero() || publicID == "" {
		return auth.AdminTokenRecord{}, errors.New("invalid administrator token revocation scope")
	}
	var record auth.AdminTokenRecord
	err := s.db.WithTxProbe(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(0x5552444e41444d49)); err != nil {
			return classifyError(err)
		}
		var available int
		if err := tx.QueryRow(ctx, `
            SELECT count(*)
            FROM admin_tokens t
            JOIN admin_principals p ON p.id = t.principal_id
            WHERE t.revoked_at IS NULL AND t.expires_at > $1 AND p.status = 'active'`, now).Scan(&available); err != nil {
			return classifyError(err)
		}
		if available <= 1 {
			return ErrLastAdminToken
		}
		return revokeAdminTokenTx(ctx, tx, actor, principal, publicID, now, &record)
	}, func(ctx context.Context) (bool, error) {
		var revoked bool
		err := s.db.Pool().QueryRow(ctx, `
            SELECT EXISTS (SELECT 1 FROM admin_tokens WHERE principal_id = $1 AND public_id = $2 AND revoked_at IS NOT NULL)`,
			uuidArg(principal), publicID).Scan(&revoked)
		return revoked, err
	})
	if err != nil {
		return auth.AdminTokenRecord{}, err
	}
	return record, nil
}

func revokeAdminTokenTx(ctx context.Context, tx pgx.Tx, actor, principal domain.PrincipalID, publicID string, now time.Time, record *auth.AdminTokenRecord) error {
	var (
		tokenID, principalID pgtype.UUID
		digest               []byte
		pepperID             string
		expiresAt            time.Time
		revokedAt            *time.Time
		authVersion          uint64
	)
	err := tx.QueryRow(ctx, `
        UPDATE admin_tokens SET revoked_at = $1, auth_version = auth_version + 1
        WHERE principal_id = $2 AND public_id = $3 AND revoked_at IS NULL
        RETURNING id, principal_id, public_id, secret_digest, digest_key_id,
                  expires_at, revoked_at, auth_version`,
		now, uuidArg(principal), publicID).Scan(
		&tokenID, &principalID, &record.PublicID, &digest, &pepperID, &expiresAt, &revokedAt, &authVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return classifyError(err)
	}
	record.ID = domain.UUID(tokenID.Bytes)
	record.PrincipalID = domain.PrincipalID(domain.UUID(principalID.Bytes))
	record.Digest = append([]byte(nil), digest...)
	record.PepperID = pepperID
	record.ExpiresAt = expiresAt
	record.RevokedAt = revokedAt
	record.AuthVersion = authVersion
	return insertAuditEvent(ctx, tx, auditEvent{
		actor: actor, action: auditActionAdminTokenRevoke,
		targetType: auditTargetAdminToken, targetID: record.ID, result: auditResultSuccess,
		metadata: map[string]string{"auth_version": strconv.FormatUint(authVersion, 10)},
	})
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

func validatePublicKeyScopes(scopes []string) error {
	if len(scopes) == 0 {
		return errors.New("api key scopes must not be empty")
	}
	for _, scope := range scopes {
		switch domain.PermissionScope(scope) {
		case domain.ScopeModelsRead, domain.ScopeModelsInvoke, domain.ScopeUsageRead:
		default:
			return errors.New("api key scope is not an inference scope")
		}
	}
	return nil
}
