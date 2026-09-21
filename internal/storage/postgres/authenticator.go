package postgres

import (
	"context"
	"errors"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/clock"
	"example.com/urbino/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Authenticator reads identity records from PostgreSQL on every request. This
// intentionally avoids a process-local authoritative credential map: revoke,
// disable and bootstrap state are shared by all serving instances.
type Authenticator struct {
	db     *DB
	pepper auth.Pepper
	clock  clock.Clock
}

func NewAuthenticator(db *DB, pepper auth.Pepper, c clock.Clock) (*Authenticator, error) {
	if db == nil || db.Pool() == nil || pepper.ID == "" || len(pepper.Key) < 32 {
		return nil, errors.New("authenticator configuration is invalid")
	}
	if c == nil {
		c = clock.Real{}
	}
	return &Authenticator{db: db, pepper: pepper, clock: c}, nil
}

func (a *Authenticator) AuthenticatePublic(ctx context.Context, req auth.Request) (auth.Principal, error) {
	credential, err := auth.ParseHeaders(req.Header, req.Query, auth.PublicListener)
	if err != nil {
		return auth.Principal{}, err
	}
	publicID, secret, err := auth.ParsePublicCredential(credential.Value)
	if err != nil {
		return auth.Principal{}, auth.ErrInvalidCredential
	}
	record, err := a.lookupPublic(ctx, publicID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.Principal{}, auth.ErrInvalidCredential
		}
		return auth.Principal{}, classifyError(err)
	}
	if err := auth.Verify(record, secret, a.pepper, a.clock.Now()); err != nil {
		return auth.Principal{}, err
	}
	return auth.Principal{
		TenantID:    record.Tenant,
		ProjectID:   record.Project,
		UserID:      record.User,
		KeyID:       record.PublicID,
		Scopes:      append([]string(nil), record.Scopes...),
		Models:      append([]string(nil), record.Models...),
		AuthVersion: int64(record.AuthVersion),
	}, nil
}

func (a *Authenticator) AuthenticateAdmin(ctx context.Context, req auth.Request) (auth.AdminPrincipal, error) {
	credential, err := auth.ParseHeaders(req.Header, req.Query, auth.AdminListener)
	if err != nil {
		return auth.AdminPrincipal{}, err
	}
	publicID, secret, err := auth.ParseAdminCredential(credential.Value)
	if err != nil {
		return auth.AdminPrincipal{}, auth.ErrInvalidCredential
	}
	record, principal, err := a.lookupAdmin(ctx, publicID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.AdminPrincipal{}, auth.ErrInvalidCredential
		}
		return auth.AdminPrincipal{}, classifyError(err)
	}
	if err := auth.VerifyAdmin(record, secret, a.pepper, a.clock.Now()); err != nil {
		return auth.AdminPrincipal{}, err
	}
	scopes := make([]string, 0, len(principal.scopes))
	for _, scope := range principal.scopes {
		scopes = append(scopes, scope)
	}
	return auth.AdminPrincipal{AdminID: principal.id, Scopes: scopes}, nil
}

func (a *Authenticator) lookupPublic(ctx context.Context, publicID string) (auth.KeyRecord, error) {
	var (
		id, tenantID, projectID, userID pgtype.UUID
		digest                          []byte
		pepperID, status                string
		scopes, models                  []string
		expiresAt                       time.Time
		revokedAt                       *time.Time
		authVersion                     uint64
	)
	err := a.db.Pool().QueryRow(ctx, `
		SELECT k.id, k.tenant_id, k.project_id, k.user_id, k.secret_digest,
		       k.digest_key_id, k.scopes, k.allowed_models, k.status,
		       k.expires_at, k.revoked_at, k.auth_version
		FROM api_keys k
		JOIN tenants t ON t.id = k.tenant_id AND t.status = 'active'
		JOIN projects p ON p.tenant_id = k.tenant_id AND p.id = k.project_id AND p.status = 'active'
		JOIN users u ON u.tenant_id = k.tenant_id AND u.id = k.user_id AND u.status = 'active'
		JOIN project_members m ON m.tenant_id = k.tenant_id AND m.project_id = k.project_id
		  AND m.user_id = k.user_id AND m.status = 'active'
		WHERE k.public_id = $1`, publicID).Scan(
		&id, &tenantID, &projectID, &userID, &digest, &pepperID, &scopes, &models,
		&status, &expiresAt, &revokedAt, &authVersion)
	if err != nil {
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

type adminPrincipalRow struct {
	id     domain.PrincipalID
	scopes []string
}

func (a *Authenticator) lookupAdmin(ctx context.Context, publicID string) (auth.AdminTokenRecord, adminPrincipalRow, error) {
	var (
		tokenID, principalID pgtype.UUID
		digest               []byte
		pepperID             string
		expiresAt            time.Time
		revokedAt            *time.Time
		authVersion          uint64
		status               string
		scopes               []string
	)
	err := a.db.Pool().QueryRow(ctx, `
		SELECT t.id, t.principal_id, t.secret_digest, t.digest_key_id,
		       t.expires_at, t.revoked_at, t.auth_version,
		       p.status, p.scopes
		FROM admin_tokens t
		JOIN admin_principals p ON p.id = t.principal_id
		WHERE t.public_id = $1 AND p.status = 'active'`, publicID).Scan(
		&tokenID, &principalID, &digest, &pepperID, &expiresAt, &revokedAt,
		&authVersion, &status, &scopes)
	if err != nil {
		return auth.AdminTokenRecord{}, adminPrincipalRow{}, err
	}
	return auth.AdminTokenRecord{
		ID: domain.UUID(tokenID.Bytes), PrincipalID: domain.PrincipalID(domain.UUID(principalID.Bytes)),
		PublicID: publicID, Digest: append([]byte(nil), digest...), PepperID: pepperID,
		ExpiresAt: expiresAt, RevokedAt: revokedAt, AuthVersion: authVersion,
	}, adminPrincipalRow{id: domain.PrincipalID(domain.UUID(principalID.Bytes)), scopes: scopes}, nil
}
