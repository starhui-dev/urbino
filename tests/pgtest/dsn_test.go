package pgtest

import (
	"strings"
	"testing"
)

func TestValidateDSN(t *testing.T) {
	for _, raw := range []string{
		"postgres://urbino_test_admin:synthetic-password@127.0.0.1:55432/urbino_test?sslmode=disable",
		"postgresql://urbino_test_admin:synthetic-password@[::1]:55432/urbino_test?sslmode=disable",
	} {
		if _, err := ValidateDSN(raw); err != nil {
			t.Fatal("安全的本地测试 DSN 被拒绝")
		}
	}
}

func TestValidateDSNRejectsUnsafeTargetsAndRedacts(t *testing.T) {
	for name, raw := range map[string]string{
		"missing": "",
		"keyword-dsn": "host=127.0.0.1 dbname=urbino_test user=urbino_test_admin password=synthetic-secret",
		"remote-ip": "postgres://urbino_test_admin:synthetic-secret@192.0.2.1:55432/urbino_test?sslmode=disable",
		"hostname": "postgres://urbino_test_admin:synthetic-secret@localhost:55432/urbino_test?sslmode=disable",
		"business-database": "postgres://urbino_test_admin:synthetic-secret@127.0.0.1:55432/urbino?sslmode=disable",
		"business-user": "postgres://postgres:synthetic-secret@127.0.0.1:55432/urbino_test?sslmode=disable",
		"implicit-password": "postgres://urbino_test_admin@127.0.0.1:55432/urbino_test?sslmode=disable",
		"implicit-port": "postgres://urbino_test_admin:synthetic-secret@127.0.0.1/urbino_test?sslmode=disable",
		"ssl-fallback": "postgres://urbino_test_admin:synthetic-secret@127.0.0.1:55432/urbino_test?sslmode=prefer",
		"query-host-override": "postgres://urbino_test_admin:synthetic-secret@127.0.0.1:55432/urbino_test?sslmode=disable&host=192.0.2.1",
		"query-hostaddr": "postgres://urbino_test_admin:synthetic-secret@127.0.0.1:55432/urbino_test?sslmode=disable&hostaddr=192.0.2.1",
		"service": "postgres://urbino_test_admin:synthetic-secret@127.0.0.1:55432/urbino_test?sslmode=disable&service=production",
		"options": "postgres://urbino_test_admin:synthetic-secret@127.0.0.1:55432/urbino_test?sslmode=disable&options=-csearch_path=production",
		"duplicate-sslmode": "postgres://urbino_test_admin:synthetic-secret@127.0.0.1:55432/urbino_test?sslmode=disable&sslmode=require",
		"fragment": "postgres://urbino_test_admin:synthetic-secret@127.0.0.1:55432/urbino_test?sslmode=disable#fragment",
		"malformed-escape": "postgres://urbino_test_admin:synthetic-secret%zz@127.0.0.1:55432/urbino_test?sslmode=disable",
		"encoded-path": "postgres://urbino_test_admin:synthetic-secret@127.0.0.1:55432/%75rbino_test?sslmode=disable",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ValidateDSN(raw)
			if err == nil {
				t.Fatal("不安全测试目标未被拒绝")
			}
			if strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), raw) && raw != "" {
				t.Fatal("测试 DSN 错误泄漏输入")
			}
		})
	}
}

func TestValidateEnvironment(t *testing.T) {
	if err := ValidateEnvironment([]string{"PATH=C:\\bin", "URBINO_TEST_DATABASE_URL=synthetic"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"PGSERVICE", "PGSERVICEFILE", "PGPASSFILE", "PGHOST", "PGOPTIONS", "pgpassword"} {
		t.Run(name, func(t *testing.T) {
			err := ValidateEnvironment([]string{name + "=synthetic-secret"})
			if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("隐式 PostgreSQL 配置没有安全拒绝")
			}
		})
	}
}
