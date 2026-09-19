package postgres

import (
	"context"
	"errors"
	"os"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/domain"
	"github.com/jackc/pgx/v5"
)

const bootstrapLockKey int64 = 0x5552424e424f4f54 // URBNBOOT

// BootstrapAdmin creates the first management principal and token atomically.
// The token is written with O_EXCL before the commit; a failed transaction
// removes that file and never exposes the secret in an error.
func BootstrapAdmin(ctx context.Context, db *DB, issuer auth.Issuer, outputPath string, now time.Time) error {
	if db == nil || db.Pool() == nil || outputPath == "" {
		return errors.New("bootstrap configuration is invalid")
	}
	principalID, err := domain.NewUUID()
	if err != nil {
		return errors.New("bootstrap identity generation failed")
	}
	token, err := issuer.IssueAdmin([]domain.PermissionScope{
		domain.ScopeTenantsRead, domain.ScopeTenantsWrite, domain.ScopeModelsRead, domain.ScopeUsageRead,
	}, now, 24*time.Hour)
	if err != nil {
		return errors.New("bootstrap token generation failed")
	}
	if err := auth.WriteSecretExclusive(outputPath, token.Value); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(outputPath)
		}
	}()
	if err := db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", bootstrapLockKey); err != nil {
			return errors.New("bootstrap transaction failed")
		}
		var count int64
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM admin_principals").Scan(&count); err != nil {
			return errors.New("bootstrap transaction failed")
		}
		if count != 0 {
			return errors.New("administrator bootstrap already completed")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_principals (id, display_name, status, scopes, version)
			VALUES ($1, 'bootstrap administrator', 'active', $2, 1)`,
			uuidArg(principalID), []string{string(domain.ScopeTenantsRead), string(domain.ScopeTenantsWrite), string(domain.ScopeModelsRead), string(domain.ScopeUsageRead)}); err != nil {
			return errors.New("bootstrap transaction failed")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_tokens (id, principal_id, public_id, secret_digest, digest_key_id, expires_at, auth_version)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			uuidArg(token.Record.ID), uuidArg(principalID), token.Record.PublicID, token.Record.Digest,
			token.Record.PepperID, token.Record.ExpiresAt, token.Record.AuthVersion); err != nil {
			return errors.New("bootstrap transaction failed")
		}
		return nil
	}); err != nil {
		return err
	}
	committed = true
	return nil
}
