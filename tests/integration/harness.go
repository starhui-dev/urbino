// Package integration contains the phase 02 persistence test harness.
//
// harness.go is deliberately free of production-package and driver imports:
// it only implements the local test DSN safety gate, DSN redaction and pure
// SQL text used by the introspection helpers. Everything that talks to
// PostgreSQL lives in the _test.go files of this directory, so that
// tests/persistence can unit-test the safety gate before the storage
// implementation is merged.
//
// Security contract of the gate: integration tests never derive a connection
// target from any default, config file or well-known local socket. The only
// accepted source is the explicit URBINO_TEST_DATABASE_DSN environment
// variable, and even then the DSN must pass every safety rule below before a
// test may open a connection.
package integration

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TestDSNEnv is the only environment variable the integration tests read for
// a database target. It must point at a disposable local test database.
const TestDSNEnv = "URBINO_TEST_DATABASE_DSN"

// TestDSN is a parsed, safety-checked DSN. Raw is never included in error
// messages or logs; use RedactDSN when a DSN shape must be shown.
type TestDSN struct {
	Raw      string
	Host     string
	Port     string
	Database string
	User     string
	urlForm  bool
	values   map[string]string
}

// ResolveTestDSN reads getenv (normally os.Getenv) and returns either a safe
// test DSN or the reason it must not be used. ok is true only when every
// safety rule passed. Callers map !ok to a distinguishable NOT_RUN outcome.
func ResolveTestDSN(getenv func(string) string) (TestDSN, string, bool) {
	raw := strings.TrimSpace(getenv(TestDSNEnv))
	if raw == "" {
		return TestDSN{}, fmt.Sprintf("environment variable %s is not set (tests never guess a database target)", TestDSNEnv), false
	}
	dsn, reason := parseTestDSN(raw)
	if reason != "" {
		return TestDSN{}, reason, false
	}
	if reason := checkSafeTestDSN(dsn); reason != "" {
		return TestDSN{}, reason, false
	}
	return dsn, "", true
}

// parseTestDSN accepts URL form (postgres://user:pass@host:port/db?opts) and
// keyword/value form (host=... port=... dbname=... user=... password=...).
// It returns the reason the DSN cannot even be parsed, or "" on success.
func parseTestDSN(raw string) (TestDSN, string) {
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return TestDSN{}, "DSN is not a parseable URL form"
		}
		switch u.Scheme {
		case "postgres", "postgresql":
		default:
			return TestDSN{}, fmt.Sprintf("DSN scheme %q is not a PostgreSQL scheme", u.Scheme)
		}
		db := strings.TrimPrefix(u.Path, "/")
		values := map[string]string{}
		for k, vs := range u.Query() {
			if len(vs) > 0 {
				values[strings.ToLower(k)] = vs[0]
			}
		}
		if _, ok := values["host"]; ok {
			return TestDSN{}, "DSN URL host query override is not allowed"
		}
		if reason := rejectRoutingOverrides(values); reason != "" {
			return TestDSN{}, reason
		}
		host := u.Hostname()
		if host == "" {
			return TestDSN{}, "DSN URL must include an explicit loopback TCP host"
		}
		port := u.Port()
		if port == "" {
			port = firstNonEmpty(values["port"], "5432")
		}
		return TestDSN{
			Raw: raw, Host: strings.ToLower(host), Port: port,
			Database: db, User: u.User.Username(),
			urlForm: true, values: values,
		}, ""
	}
	values, err := parseKeywordValueDSN(raw)
	if err != nil {
		return TestDSN{}, "DSN is not parseable keyword/value form"
	}
	if strings.TrimSpace(values["host"]) == "" {
		return TestDSN{}, "DSN keyword/value form must include an explicit loopback TCP host"
	}
	if reason := rejectRoutingOverrides(values); reason != "" {
		return TestDSN{}, reason
	}
	return TestDSN{
		Raw:      raw,
		Host:     strings.ToLower(values["host"]),
		Port:     firstNonEmpty(values["port"], "5432"),
		Database: values["dbname"],
		User:     values["user"],
		values:   values,
	}, ""
}

func rejectRoutingOverrides(values map[string]string) string {
	for _, key := range []string{"host", "hostaddr", "service", "socketdir", "unix_socket_direct"} {
		if _, ok := values[key]; ok && key != "host" {
			return fmt.Sprintf("DSN option %q is not allowed for integration targets", key)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseKeywordValueDSN(raw string) (map[string]string, error) {
	values := map[string]string{}
	for _, pair := range strings.Fields(raw) {
		k, v, ok := strings.Cut(pair, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("bad DSN pair")
		}
		v = strings.Trim(v, "'")
		values[strings.ToLower(k)] = v
	}
	return values, nil
}

// checkSafeTestDSN enforces the local-test-only contract. Each rejection
// reason is written so evidence can quote it verbatim.
func checkSafeTestDSN(dsn TestDSN) string {
	if strings.ContainsAny(dsn.Host, "/\\") || strings.HasPrefix(dsn.Host, ".") {
		return fmt.Sprintf("DSN host %q is not a loopback TCP host", RedactDSN(dsn.Host))
	}
	switch dsn.Host {
	case "127.0.0.1", "::1", "localhost":
	default:
		return fmt.Sprintf("DSN host %q is not loopback (127.0.0.1, ::1 or localhost only)", RedactDSN(dsn.Host))
	}
	for _, suspect := range []string{dsn.Host, dsn.Database, dsn.User} {
		if strings.Contains(strings.ToLower(suspect), "prod") {
			return "DSN references a production-looking host/database/user name"
		}
	}
	if dsn.Database == "" {
		return "DSN has no database name"
	}
	if !strings.HasSuffix(dsn.Database, "_test") {
		return fmt.Sprintf("test database name must end with _test, got %q", RedactDSN(dsn.Database))
	}
	switch dsn.Database {
	case "postgres", "template0", "template1":
		return "system databases are never valid integration test targets"
	}
	return ""
}

// RedactDSN replaces every password with a fixed marker and never returns the
// input unchanged. It is safe to call with any string.
func RedactDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	if strings.Contains(dsn, "://") {
		if u, err := url.Parse(dsn); err == nil {
			if u.User != nil {
				if _, hasPass := u.User.Password(); hasPass {
					u.User = url.UserPassword(u.User.Username(), "REDACTED")
				}
			}
			return u.String()
		}
		return "redacted-dsn"
	}
	redacted := dsn
	for _, pair := range strings.Fields(dsn) {
		if k, v, ok := strings.Cut(pair, "="); ok && strings.EqualFold(k, "password") {
			redacted = strings.Replace(redacted, pair, "password=REDACTED", 1)
			_ = v
		}
	}
	return redacted
}

// IsURLForm reports whether the resolved DSN was postgres://-style.
func (d TestDSN) IsURLForm() bool { return d.urlForm }

// Option returns a keyword/value option, mirroring URL query parameters.
func (d TestDSN) Option(key string) string { return d.values[strings.ToLower(key)] }

// WithDatabase returns a DSN pointing at another database on the same
// instance, keeping the mandatory _test suffix contract in the caller's hands.
func (d TestDSN) WithDatabase(name string) (string, error) {
	if d.urlForm {
		u, err := url.Parse(d.Raw)
		if err != nil {
			return "", fmt.Errorf("re-parse stored DSN: %w", err)
		}
		u.Path = "/" + name
		return u.String(), nil
	}
	fields := make([]string, 0, len(d.values))
	replaced := false
	for k, v := range d.values {
		if k == "dbname" {
			v, replaced = name, true
		}
		fields = append(fields, k+"="+v)
	}
	if !replaced {
		fields = append(fields, "dbname="+name)
	}
	sort.Strings(fields)
	return strings.Join(fields, " "), nil
}

// WithCredentials returns a DSN for the same database under another role,
// used to exercise privilege boundaries as the restricted application role.
func (d TestDSN) WithCredentials(user, password string) (string, error) {
	if d.urlForm {
		u, err := url.Parse(d.Raw)
		if err != nil {
			return "", fmt.Errorf("re-parse stored DSN: %w", err)
		}
		u.User = url.UserPassword(user, password)
		return u.String(), nil
	}
	fields := make([]string, 0, len(d.values)+1)
	for k, v := range d.values {
		switch strings.ToLower(k) {
		case "user":
			v = user
		case "password":
			v = password
		}
		fields = append(fields, k+"="+v)
	}
	if _, ok := d.values["user"]; !ok {
		fields = append(fields, "user="+user)
	}
	if _, ok := d.values["password"]; !ok {
		fields = append(fields, "password="+password)
	}
	sort.Strings(fields)
	return strings.Join(fields, " "), nil
}

// DatabaseNamed returns the sibling database name with the given suffix
// replacing the mandatory _test ending (e.g. urbino_test + "_race" ->
// urbino_test_race). The result keeps the _test suffix so scratch databases
// created by the suite also satisfy the safety contract.
func (d TestDSN) DatabaseNamed(suffix string) string {
	return strings.TrimSuffix(d.Database, "_test") + "_test" + suffix
}

// FindRepoRoot walks up from start until it finds go.mod declaring the Urbino
// module, so tests work from any working directory.
func FindRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module example.com/urbino") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("urbino module root not found above %s", start)
		}
		dir = parent
	}
}

// ListMigrationFiles returns the sorted migration SQL files of the repository,
// using only the file names (contents are applied by the production migrate
// command, never re-implemented here). It fails when the phase 02 migrations
// are absent so callers produce a real failure instead of a silent skip.
func ListMigrationFiles(repoRoot string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "migrations"))
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no migration files under migrations/ (phase 02 migrations not present)")
	}
	sort.Strings(files)
	return files, nil
}

// Introspection SQL used by the fixture. Kept as plain constants so the pure
// harness has no driver dependency; the _test.go files execute them.

// CompositeFKsSQL lists every foreign key of table with the referenced table
// and the (child, parent) column pairs, ordered per constraint.
const CompositeFKsSQL = `
SELECT tc.constraint_name,
       parent_kcu.table_name AS parent_table,
       kcu.column_name AS child_column,
       parent_kcu.column_name AS parent_column,
       kcu.ordinal_position
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_name = tc.constraint_name AND kcu.constraint_schema = tc.table_schema
JOIN information_schema.referential_constraints rc
  ON rc.constraint_name = tc.constraint_name AND rc.constraint_schema = tc.table_schema
JOIN information_schema.key_column_usage parent_kcu
  ON parent_kcu.constraint_name = rc.unique_constraint_name
 AND parent_kcu.constraint_schema = rc.unique_constraint_schema
 AND parent_kcu.ordinal_position = kcu.ordinal_position
WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_name = $1
ORDER BY tc.constraint_name, kcu.ordinal_position`

// RequiredColumnsSQL lists NOT NULL columns without default that an INSERT
// must supply for a table.
const RequiredColumnsSQL = `
SELECT column_name, data_type, udt_name
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1
  AND is_nullable = 'NO' AND column_default IS NULL
ORDER BY ordinal_position`

// TableColumnsSQL lists every public column of a table with its type.
const TableColumnsSQL = `
SELECT column_name, data_type, udt_name, is_nullable, column_default,
       character_maximum_length
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1
ORDER BY ordinal_position`

// UniqueIndexDefsSQL returns the CREATE UNIQUE index definitions of a table.
const UniqueIndexDefsSQL = `
SELECT indexname, indexdef
FROM pg_indexes
WHERE schemaname = 'public' AND tablename = $1 AND indexdef ILIKE 'CREATE UNIQUE%'`

// HistoryTableSQL finds candidate migration history tables by name. The
// production contract requires an independent schema history table; the tests
// locate it by shape (a name mentioning version/history/migration/schema) and
// fail loudly when none exists.
const HistoryTableSQL = `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = 'public'
  AND (table_name ILIKE '%version%' OR table_name ILIKE '%migration%'
       OR table_name ILIKE '%schema%' OR table_name ILIKE '%history%')
ORDER BY table_name`

// SchemaSnapshotSQL dumps the public DDL surface in a stable order so a
// before/after comparison can prove that no object changed.
const SchemaSnapshotSQL = `
SELECT 'table:' || table_name
FROM information_schema.tables WHERE table_schema = 'public'
UNION ALL
SELECT 'column:' || table_name || '.' || column_name || ':' || data_type || ':' || is_nullable
FROM information_schema.columns WHERE table_schema = 'public'
UNION ALL
SELECT 'constraint:' || constraint_name || ':' || constraint_type || ':' || table_name
FROM information_schema.table_constraints WHERE table_schema = 'public'
UNION ALL
SELECT 'index:' || indexname || ':' || indexdef
FROM pg_indexes WHERE schemaname = 'public'
ORDER BY 1`
