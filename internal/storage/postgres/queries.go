package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is the small interface emitted by sqlc for a pool or transaction.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Queries contains tenant-scoped SQL. Keep generated code in this package so
// handlers cannot accidentally issue unscoped queries.
type Queries struct{ db DBTX }

func NewQueries(db DBTX) *Queries { return &Queries{db: db} }

type RequestRow struct {
	ID, TenantID, ProjectID            uuid.UUID
	RequestID, Model, Protocol, Status string
	StartedAt                          time.Time
}

func (q *Queries) GetRequest(ctx context.Context, tenantID, id uuid.UUID) (RequestRow, error) {
	var row RequestRow
	err := q.db.QueryRow(ctx, `SELECT id, tenant_id, project_id, request_id, model, protocol, status, started_at FROM urbino.requests WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&row.ID, &row.TenantID, &row.ProjectID, &row.RequestID, &row.Model, &row.Protocol, &row.Status, &row.StartedAt)
	return row, err
}

func (q *Queries) CreateAttempt(ctx context.Context, id, requestID, tenantID uuid.UUID, attemptNo int32, dispatchPhase, resultClass, status string) (uuid.UUID, error) {
	var got uuid.UUID
	err := q.db.QueryRow(ctx, `INSERT INTO urbino.request_attempts (id,request_id,tenant_id,attempt_no,dispatch_phase,result_class,status) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, id, requestID, tenantID, attemptNo, dispatchPhase, resultClass, status).Scan(&got)
	return got, err
}
