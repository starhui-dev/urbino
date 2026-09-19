// Synthetic-data fixture for the P02 integration tests.
//
// seed() inserts one row into a table by introspecting information_schema:
// required columns (NOT NULL, no default) are filled with synthetic values,
// CHECK-domain columns come from fixtureValues, and composite foreign keys
// are satisfied by recursively seeding the parent row first (child columns
// sharing a name with a decided child value — typically tenant_id — inherit
// it, keeping every generated row inside the intended tenant scope). Tests
// override key columns explicitly; a fully overridden foreign key is left
// alone, which is exactly how T01 plants a cross-tenant violation.
//
// No fixture value is a secret: everything here is synthetic and safe to
// commit, log and quote in evidence.
package integration_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"example.com/urbino/tests/integration"
)

// fixtureValues pins columns whose values are constrained by CHECK domains
// the type-based synthesizer cannot guess.
var fixtureValues = map[string]map[string]any{
	"tenants":          {"status": "active", "currency": "USD"},
	"projects":         {"status": "active"},
	"users":            {"status": "active"},
	"price_versions":   {"status": "draft", "currency": "USD"},
	"billing_accounts": {"currency": "USD", "posted_balance_micros": int64(1000), "held_micros": int64(0)},
	"settlements":      {"status": "pending"},
}

var synthCounter atomic.Int64

type columnMeta struct {
	name       string
	dataType   string
	udt        string
	nullable   bool
	hasDefault bool
	maxLen     int
}

// fkPair is one (child, parent) column of a foreign key.
type fkPair struct{ child, parent string }

// fkConstraint is a foreign key with its ordered column pairs.
type fkConstraint struct {
	name   string
	parent string
	pairs  []fkPair
}

type fixture struct {
	t    *testing.T
	ctx  context.Context
	e    *p02Env
	conn *pgx.Conn
}

func newFixture(t *testing.T) *fixture {
	e := requireRealPG(t)
	return &fixture{t: t, ctx: t.Context(), e: e, conn: e.admin}
}

// withConn returns a fixture that seeds through another connection, e.g. the
// restricted application role used by T04.
func (f *fixture) withConn(conn *pgx.Conn) *fixture {
	return &fixture{t: f.t, ctx: f.ctx, e: f.e, conn: conn}
}

// seed inserts one synthetic row and fails the test on any error.
func (f *fixture) seed(table string, overrides map[string]any) map[string]any {
	f.t.Helper()
	row, err := f.trySeed(table, overrides, 0)
	if err != nil {
		f.t.Fatalf("seed %s: %s", table, integration.RedactDSN(err.Error()))
	}
	return row
}

// trySeed is seed returning the error instead of failing the test; T01 and
// T02 use it to plant constraint violations on purpose.
func (f *fixture) trySeed(table string, overrides map[string]any, depth int) (map[string]any, error) {
	if depth > 8 {
		return nil, fmt.Errorf("seed recursion exceeded at %s (circular foreign keys?)", table)
	}
	cols, err := f.columns(table)
	if err != nil {
		return nil, err
	}
	required := map[string]columnMeta{}
	for _, c := range cols {
		if !c.nullable && !c.hasDefault {
			required[c.name] = c
		}
	}
	final := map[string]any{}
	for name, value := range fixtureValues[table] {
		if _, ok := final[name]; !ok {
			final[name] = value
		}
	}
	for name, value := range lowerKeys(overrides) {
		final[name] = value
	}
	fks, err := f.foreignKeys(table)
	if err != nil {
		return nil, err
	}
	for _, fk := range fks {
		if fk.parent == table {
			return nil, fmt.Errorf("self-referencing FK %s on %s is not supported by the seeder", fk.name, table)
		}
		fullyDecided := true
		for _, p := range fk.pairs {
			if _, ok := final[p.child]; !ok {
				fullyDecided = false
				break
			}
		}
		if fullyDecided {
			continue
		}
		parentOverrides := map[string]any{}
		for _, p := range fk.pairs {
			if value, ok := final[p.child]; ok {
				parentOverrides[p.parent] = value
			}
		}
		parentRow, err := f.trySeed(fk.parent, parentOverrides, depth+1)
		if err != nil {
			return nil, fmt.Errorf("seed parent %s for %s.%s: %w", fk.parent, table, fk.name, err)
		}
		for _, p := range fk.pairs {
			if _, ok := overrides[p.child]; ok {
				continue
			}
			value, ok := parentRow[p.parent]
			if !ok {
				return nil, fmt.Errorf("parent %s row did not return column %s", fk.parent, p.parent)
			}
			final[p.child] = value
		}
	}
	var insertCols []string
	for name, meta := range required {
		if _, ok := final[name]; ok {
			insertCols = append(insertCols, name)
			continue
		}
		if pinned, ok := fixtureValues[table][name]; ok {
			final[name] = pinned
			insertCols = append(insertCols, name)
			continue
		}
		value, err := synthValue(table, meta)
		if err != nil {
			return nil, err
		}
		final[name] = value
		insertCols = append(insertCols, name)
	}
	for name := range lowerKeys(overrides) {
		if _, isRequired := required[name]; !isRequired {
			insertCols = append(insertCols, name)
		}
	}
	if len(insertCols) == 0 {
		return nil, fmt.Errorf("no columns resolved for insert into %s", table)
	}
	sort.Strings(insertCols)
	args := make([]any, 0, len(insertCols))
	placeholders := make([]string, 0, len(insertCols))
	for i, name := range insertCols {
		args = append(args, final[name])
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING *",
		quoteIdent(table), quoteJoin(insertCols), strings.Join(placeholders, ", "))
	rows, err := f.conn.Query(f.ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("INSERT INTO %s returned no rows", table)
	}
	fields := rows.FieldDescriptions()
	values := make([]any, len(fields))
	ptrs := make([]any, len(fields))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	row := map[string]any{}
	for i, fd := range fields {
		row[strings.ToLower(fd.Name)] = values[i]
	}
	return row, nil
}

func (f *fixture) columns(table string) ([]columnMeta, error) {
	rows, err := f.conn.Query(f.ctx, integration.TableColumnsSQL, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []columnMeta
	for rows.Next() {
		var c columnMeta
		var nullable string
		var def *string
		var maxLen *int32
		if err := rows.Scan(&c.name, &c.dataType, &c.udt, &nullable, &def, &maxLen); err != nil {
			return nil, err
		}
		c.nullable = nullable == "YES"
		c.hasDefault = def != nil
		if maxLen != nil {
			c.maxLen = int(*maxLen)
		}
		cols = append(cols, c)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("table %s has no columns (missing from migrated schema?)", table)
	}
	return cols, rows.Err()
}

// foreignKeys returns the table's foreign keys with aligned child/parent
// column pairs, used both by the seeder and by T01's composite-key contract
// assertion.
func (f *fixture) foreignKeys(table string) ([]fkConstraint, error) {
	rows, err := f.conn.Query(f.ctx, integration.CompositeFKsSQL, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byName := map[string]*fkConstraint{}
	var order []string
	for rows.Next() {
		var name, parent, child, parentCol string
		var ordinal int32
		if err := rows.Scan(&name, &parent, &child, &parentCol, &ordinal); err != nil {
			return nil, err
		}
		fk, ok := byName[name]
		if !ok {
			fk = &fkConstraint{name: name, parent: parent}
			byName[name] = fk
			order = append(order, name)
		}
		fk.pairs = append(fk.pairs, fkPair{child: child, parent: parentCol})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]fkConstraint, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out, nil
}

// uniqueIndexColumns returns the column lists of every unique index on table,
// e.g. ["tenant_id, idempotency_digest"], for contract presence assertions.
func (f *fixture) uniqueIndexColumns(table string) ([][]string, error) {
	rows, err := f.conn.Query(f.ctx, integration.UniqueIndexDefsSQL, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var defs []string
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			return nil, err
		}
		defs = append(defs, def)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out [][]string
	for _, def := range defs {
		open := strings.LastIndex(def, "(")
		close_ := strings.LastIndex(def, ")")
		if open < 0 || close_ <= open {
			return nil, fmt.Errorf("cannot parse unique index definition: %s", def)
		}
		var cols []string
		for _, col := range strings.Split(def[open+1:close_], ",") {
			cols = append(cols, strings.ToLower(strings.Trim(strings.TrimSpace(col), `"`)))
		}
		out = append(out, cols)
	}
	return out, nil
}

// assertUniqueCovers fails unless table has a unique index over exactly the
// given columns.
func (f *fixture) assertUniqueCovers(table string, want ...string) {
	f.t.Helper()
	indexes, err := f.uniqueIndexColumns(table)
	if err != nil {
		f.t.Fatalf("list unique indexes on %s: %s", table, integration.RedactDSN(err.Error()))
	}
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	for _, cols := range indexes {
		if len(cols) != len(sorted) {
			continue
		}
		candidate := append([]string(nil), cols...)
		sort.Strings(candidate)
		if equalStrings(candidate, sorted) {
			return
		}
	}
	f.t.Fatalf("table %s has no unique index over exactly (%s); found %v", table, strings.Join(want, ", "), indexes)
}

// snapshot returns the stable DDL fingerprint used by T06.
func (f *fixture) snapshot() []string {
	f.t.Helper()
	rows, err := f.conn.Query(f.ctx, integration.SchemaSnapshotSQL)
	if err != nil {
		f.t.Fatalf("schema snapshot: %s", integration.RedactDSN(err.Error()))
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			f.t.Fatalf("scan schema snapshot: %s", integration.RedactDSN(err.Error()))
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		f.t.Fatalf("schema snapshot: %s", integration.RedactDSN(err.Error()))
	}
	return out
}

// execOK runs a statement and fails the test on error (message redacted).
func (f *fixture) execOK(stmt string, args ...any) {
	f.t.Helper()
	if _, err := f.conn.Exec(f.ctx, stmt, args...); err != nil {
		f.t.Fatalf("exec %q: %s", stmt, integration.RedactDSN(err.Error()))
	}
}

// tryExec returns the raw error of a statement, for violation tests.
func (f *fixture) tryExec(stmt string, args ...any) error {
	_, err := f.conn.Exec(f.ctx, stmt, args...)
	return err
}

// synthValue generates a synthetic value for a required column by type.
func synthValue(table string, c columnMeta) (any, error) {
	n := synthCounter.Add(1)
	switch c.udt {
	case "uuid":
		return syntheticUUID(), nil
	case "text", "varchar", "bpchar":
		base := fmt.Sprintf("syn-%s-%s-%d", table, c.name, n)
		if c.maxLen > 0 && len(base) > c.maxLen {
			base = base[:c.maxLen]
		}
		return base, nil
	case "int2", "int4", "int8":
		return n, nil
	case "float4", "float8":
		return float64(n), nil
	case "numeric":
		return int64(n), nil
	case "bool":
		return true, nil
	case "timestamptz", "timestamp", "date":
		return time.Now().UTC().Truncate(time.Millisecond), nil
	case "json", "jsonb":
		return "{}", nil
	case "bytea":
		return []byte(fmt.Sprintf("syn-%d", n)), nil
	}
	return nil, fmt.Errorf("seeder has no synthetic value for %s.%s (type %s/%s); extend fixtureValues", table, c.name, c.dataType, c.udt)
}

// syntheticUUID returns a random RFC 4122 version 4 UUID.
func syntheticUUID() [16]byte {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return b
}

// pgErrCode returns the PostgreSQL error code of err (e.g. "23503"),
// failing the test when err is not a server error.
func pgErrCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected a PostgreSQL error, got nil")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	t.Fatalf("expected a *pgconn.PgError, got %T: %s", err, integration.RedactDSN(err.Error()))
	return ""
}

func lowerKeys(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[strings.ToLower(k)] = v
	}
	return out
}

func quoteJoin(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdent(c)
	}
	return strings.Join(quoted, ", ")
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
