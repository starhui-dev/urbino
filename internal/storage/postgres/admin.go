package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const bootstrapLockKey int64 = 0x5552424e424f4f54 // URBNBOOT

// bootstrapCommit reports how a bootstrap transaction ended. Only
// bootstrapRolledBack proves that the exclusive secret file describes a token
// that cannot exist in the database.
type bootstrapCommit uint8

const (
	bootstrapRolledBack bootstrapCommit = iota
	bootstrapCommitted
	bootstrapUnknown
)

// ErrBootstrapOutcomeUnknown reports a bootstrap whose COMMIT response was
// lost. The exclusive secret file is preserved because the administrator
// token may already be committed and that file is its only copy; an operator
// must verify the administrator row before removing it.
var ErrBootstrapOutcomeUnknown = errors.New("administrator bootstrap commit outcome is unknown; the exclusive secret file was preserved")

// bootstrapScopes is the fixed authority of the first administrator. It is
// never derived from request input, so bootstrap cannot self-grant arbitrary
// permissions while still leaving the fresh installation usable.
func bootstrapScopes() []domain.PermissionScope {
	return []domain.PermissionScope{
		domain.ScopeTenantsRead, domain.ScopeTenantsWrite,
		domain.ScopeKeysRead, domain.ScopeKeysWrite,
	}
}

// BootstrapAdmin creates the first management principal and token atomically.
// The token is written with O_EXCL before the commit, and the exclusive secret
// file is removed only when the transaction is known not to have committed: an
// unknown commit outcome keeps the file, because the token may already exist
// and the file holds its only copy.
func BootstrapAdmin(ctx context.Context, db *DB, issuer auth.Issuer, outputPath string, now time.Time) error {
	if db == nil || db.Pool() == nil || outputPath == "" {
		return errors.New("bootstrap configuration is invalid")
	}
	principalID, err := domain.NewUUID()
	if err != nil {
		return errors.New("bootstrap identity generation failed")
	}
	scopes := bootstrapScopes()
	token, err := issuer.IssueAdmin(scopes, now, 24*time.Hour)
	if err != nil {
		return errors.New("bootstrap token generation failed")
	}
	if err := auth.WriteSecretExclusive(outputPath, token.Value); err != nil {
		return err
	}
	outcome, err := bootstrapTx(ctx, db, principalID, scopes, token)
	switch outcome {
	case bootstrapCommitted:
		return nil
	case bootstrapUnknown:
		return err
	default:
		_ = os.Remove(outputPath)
		return err
	}
}

// bootstrapTx runs the bootstrap writes and reports how the transaction
// ended. A failure before COMMIT is proven not to be committed: no COMMIT was
// sent and PostgreSQL aborts an open transaction when its connection ends.
func bootstrapTx(ctx context.Context, db *DB, principalID domain.UUID, scopes []domain.PermissionScope, token auth.IssuedAdminToken) (bootstrapCommit, error) {
	tx, err := db.Pool().BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return bootstrapRolledBack, classifyError(err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()
	if err := bootstrapStatements(ctx, tx, principalID, scopes, token); err != nil {
		return bootstrapRolledBack, classifyError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		committed = true
		// The server answered ROLLBACK or refused the COMMIT with an error:
		// PostgreSQL only errors on COMMIT when nothing was committed.
		var pgErr *pgconn.PgError
		if errors.Is(err, pgx.ErrTxCommitRollback) || errors.As(err, &pgErr) {
			return bootstrapRolledBack, classifyError(err)
		}
		// The COMMIT response was lost: the row may or may not exist. Probe
		// with a context that outlives a canceled caller, and never remove the
		// only copy of the secret on an unknown outcome.
		exists, probeErr := bootstrapTokenCommitted(ctx, db, token.Record.ID, principalID)
		if probeErr != nil || !exists {
			return bootstrapUnknown, ErrBootstrapOutcomeUnknown
		}
		return bootstrapCommitted, nil
	}
	committed = true
	return bootstrapCommitted, nil
}

// bootstrapStatements performs the guarded bootstrap writes: the advisory lock
// serializes concurrent bootstraps, the count rejects a second bootstrap, and
// the audit event commits with the principal and token it describes.
func bootstrapStatements(ctx context.Context, tx pgx.Tx, principalID domain.UUID, scopes []domain.PermissionScope, token auth.IssuedAdminToken) error {
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
	scopeList := scopeStrings(scopes)
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_principals (id, display_name, status, scopes, version)
		VALUES ($1, 'bootstrap administrator', 'active', $2, 1)`, uuidArg(principalID), scopeList); err != nil {
		return errors.New("bootstrap transaction failed")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_tokens (id, principal_id, public_id, secret_digest, digest_key_id, expires_at, auth_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		uuidArg(token.Record.ID), uuidArg(principalID), token.Record.PublicID, token.Record.Digest,
		token.Record.PepperID, token.Record.ExpiresAt, token.Record.AuthVersion); err != nil {
		return errors.New("bootstrap transaction failed")
	}
	if err := insertAuditEvent(ctx, tx, auditEvent{
		actor: domain.PrincipalID{}, action: auditActionAdminBootstrap,
		targetType: auditTargetAdminPrincipal, targetID: principalID, result: auditResultSuccess,
		metadata: map[string]string{"scopes": strings.Join(scopeList, ",")},
	}); err != nil {
		return errors.New("bootstrap transaction failed")
	}
	return nil
}

// bootstrapTokenCommitted reports whether the exact bootstrap token row is
// present. It probes with a detached context because the caller's context may
// already be canceled. A probe failure never proves absence: the caller must
// keep the secret file.
func bootstrapTokenCommitted(ctx context.Context, db *DB, tokenID domain.UUID, principalID domain.UUID) (bool, error) {
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var exists bool
	if err := db.Pool().QueryRow(probeCtx, `
		SELECT EXISTS (
			SELECT 1 FROM admin_tokens WHERE id = $1 AND principal_id = $2
		)`, uuidArg(tokenID), uuidArg(principalID)).Scan(&exists); err != nil {
		return false, classifyError(err)
	}
	return exists, nil
}
