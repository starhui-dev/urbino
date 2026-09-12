package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	urbinoConfig "github.com/starhui-dev/urbino/internal/config"
	"github.com/starhui-dev/urbino/internal/storage/postgres"
)

const maxDSNBytes = 16 << 10

// 迁移凭据独立于运行配置，要求操作者明确环境和数据库，避免误用应用凭据迁移。
func migrateCommand(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("dsn-file", "", "migrator DSN file")
	environment := flags.String("environment", "", "development or production")
	database := flags.String("database", "", "expected database name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *path == "" || *database == "" || (*environment != "development" && *environment != "production") {
		return errors.New("migrate: --dsn-file, --database and --environment development|production are required; positional arguments are forbidden")
	}
	dsn, err := readDSNFile(*path)
	if err != nil {
		return err
	}
	if err := validateMigrationTarget(dsn, *database); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := postgres.Migrate(ctx, dsn); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "database migration complete")
	return err
}

func validateMigrationTarget(dsn, expected string) error {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.User == nil || expected == "" {
		return errors.New("migrate: explicit PostgreSQL URL and expected database are required")
	}
	if strings.TrimPrefix(u.Path, "/") != expected || strings.Contains(expected, "/") {
		return errors.New("migrate: database does not match the declared target")
	}
	// libpq 参数可覆盖 URL 中的目标；迁移命令不接受这些二义性。
	for _, key := range []string{"dbname", "database", "host", "hostaddr", "port", "user", "service", "servicefile", "passfile"} {
		if u.Query().Has(key) {
			return errors.New("migrate: connection target override is forbidden")
		}
	}
	return nil
}

func readDSNFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("database secret file cannot be opened")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxDSNBytes+1))
	if err != nil || len(b) > maxDSNBytes {
		return "", errors.New("database secret file is unreadable or too large")
	}
	dsn := strings.TrimSpace(string(b))
	if dsn == "" {
		return "", errors.New("database secret file is empty")
	}
	return dsn, nil
}

func databaseURL(cfg urbinoConfig.Database) (string, error) {
	if cfg.URL != "" && cfg.URLFile != "" {
		return "", errors.New("database.url and database.url_file are mutually exclusive")
	}
	if cfg.URLFile != "" {
		return readDSNFile(cfg.URLFile)
	}
	return strings.TrimSpace(cfg.URL), nil
}
