package integration

import "testing"

func TestResolveTestDSNRejectsRoutingOverrides(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "url host query", raw: "postgres://user:secret@127.0.0.1:5432/urbino_test?host=evil.example"},
		{name: "url hostaddr", raw: "postgres://user:secret@127.0.0.1:5432/urbino_test?hostaddr=10.0.0.8"},
		{name: "url service", raw: "postgres://user:secret@127.0.0.1:5432/urbino_test?service=production"},
		{name: "keyword hostaddr", raw: "host=127.0.0.1 port=5432 dbname=urbino_test hostaddr=10.0.0.8"},
		{name: "keyword service", raw: "host=127.0.0.1 port=5432 dbname=urbino_test service=production"},
		{name: "keyword missing host", raw: "port=5432 dbname=urbino_test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, reason, ok := ResolveTestDSN(func(string) string { return tc.raw })
			if ok || reason == "" {
				t.Fatalf("ResolveTestDSN accepted unsafe target: ok=%v reason=%q", ok, reason)
			}
		})
	}
}

func TestResolveTestDSNAcceptsExplicitLoopbackTCP(t *testing.T) {
	for _, raw := range []string{
		"postgres://user:secret@127.0.0.1:5432/urbino_test?sslmode=disable",
		"host=localhost port=5432 dbname=urbino_test user=user password=secret",
	} {
		t.Run(raw, func(t *testing.T) {
			dsn, reason, ok := ResolveTestDSN(func(string) string { return raw })
			if !ok {
				t.Fatalf("ResolveTestDSN rejected explicit loopback target: %q", reason)
			}
			if dsn.Host == "" || dsn.Database != "urbino_test" {
				t.Fatalf("unexpected parsed DSN metadata: host=%q database=%q", dsn.Host, dsn.Database)
			}
		})
	}
}
