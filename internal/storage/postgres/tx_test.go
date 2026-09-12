package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type txTestPool struct{ tx *txTestTx }

func (p *txTestPool) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	p.tx = &txTestTx{}
	return p.tx, nil
}

type txTestTx struct {
	rollbackCalled bool
	rollbackErr    error
}

func (tx *txTestTx) Begin(context.Context) (pgx.Tx, error) { return tx, nil }
func (tx *txTestTx) Commit(context.Context) error          { return nil }
func (tx *txTestTx) Rollback(context.Context) error {
	tx.rollbackCalled = true
	return tx.rollbackErr
}
func (tx *txTestTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (tx *txTestTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (tx *txTestTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (tx *txTestTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (tx *txTestTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (tx *txTestTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (tx *txTestTx) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }
func (tx *txTestTx) Conn() *pgx.Conn                                         { return nil }

func TestRunTxRollsBackWhenCallbackPanics(t *testing.T) {
	p := &txTestPool{}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("RunTx did not preserve callback panic")
			}
		}()
		_ = RunTx(context.Background(), p, pgx.TxOptions{}, 1, func(pgx.Tx) error {
			panic("callback panic")
		})
	}()
	if p.tx == nil || !p.tx.rollbackCalled {
		t.Fatal("panic path did not rollback transaction")
	}
}

func TestRunTxReportsRollbackFailure(t *testing.T) {
	p := &txTestPool{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RunTx(ctx, p, pgx.TxOptions{}, 1, func(tx pgx.Tx) error {
		p.tx.rollbackErr = errors.New("rollback failed")
		return errors.New("callback failed")
	})
	if err == nil || err.Error() != "database transaction rollback failed" {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.tx.rollbackCalled {
		t.Fatal("callback error path did not rollback transaction")
	}
}
