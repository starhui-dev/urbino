package config

import "testing"

func TestStrictYAML(t *testing.T) {
	if _, err := LoadBytes([]byte("server:\n  nope: true\n")); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := LoadBytes([]byte("server:\n  internal:\n    listen: a\nserver:\n  admin:\n    listen: b\n")); err == nil {
		t.Fatal("duplicate key accepted")
	}
	if _, err := LoadBytes([]byte("a: &x 1\nb: *x\n")); err == nil {
		t.Fatal("alias accepted")
	}
	if _, err := LoadBytes([]byte("environment: development\n---\nenvironment: development\n")); err == nil {
		t.Fatal("multi-document accepted")
	}
}

func TestProductionGuards(t *testing.T) {
	if _, err := LoadBytes([]byte("environment: production\nsecurity:\n  test_mode: true\n")); err == nil {
		t.Fatal("production test mode accepted")
	}
	if _, err := LoadBytes([]byte("environment: production\ntransport:\n  tls_verify: false\n")); err == nil {
		t.Fatal("production insecure TLS accepted")
	}
	if _, err := LoadBytes([]byte("environment: production\nserver:\n  admin: {listen: 203.0.113.10:9090, enabled: true}\ndatabase:\n  url_file: /run/secrets/db\nvalkey:\n  url_file: /run/secrets/valkey\ntransport:\n  provider_allowlist: [openai]\nbilling:\n  mode: prepaid\n  currency: USD\n")); err == nil {
		t.Fatal("public production admin listener accepted")
	}
	if _, err := LoadBytes([]byte("environment: production\nserver:\n  admin: {listen: localhost:9090, enabled: true}\ndatabase:\n  url_file: /run/secrets/db\nvalkey:\n  url_file: /run/secrets/valkey\ntransport:\n  provider_allowlist: [openai]\nbilling:\n  mode: prepaid\n  currency: USD\n")); err == nil {
		t.Fatal("hostname production admin listener accepted")
	}
	if _, err := LoadBytes([]byte("environment: production\ntransport:\n  provider_allowlist: [openai]\nbilling:\n  mode: prepaid\n  currency: USD\n")); err == nil {
		t.Fatal("production empty secret references accepted")
	}
	if _, err := LoadBytes([]byte("environment: production\ndatabase:\n  url_file: /run/secrets/db\nvalkey:\n  url_file: /run/secrets/valkey\ntransport:\n  provider_allowlist: ['']\nbilling:\n  mode: prepaid\n  currency: USD\n")); err == nil {
		t.Fatal("empty provider allowlist entry accepted")
	}
	if _, err := LoadBytes([]byte("environment: production\ndatabase:\n  url_file: /run/secrets/db\nvalkey:\n  url_file: /run/secrets/valkey\ntransport:\n  provider_allowlist: [openai]\nbilling:\n  mode: disabled\n")); err == nil {
		t.Fatal("invalid production billing accepted")
	}
	if _, err := LoadBytes([]byte("environment: production\ndatabase:\n  url_file: /run/secrets/db\nvalkey:\n  url_file: /run/secrets/valkey\ntransport:\n  provider_allowlist: [openai]\nbilling:\n  mode: prepaid\n  currency: usd\n")); err == nil {
		t.Fatal("invalid production currency accepted")
	}
}

func TestEnvironmentContract(t *testing.T) {
	if err := ValidateEnvironment([]string{"URBINO_CONFIG=x", "URBINO_HEALTH_ADDR=:1"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEnvironment([]string{"URBINO_UNKNOWN=x"}); err == nil {
		t.Fatal("unknown env accepted")
	}
}

func TestBillingContract(t *testing.T) {
	if _, err := LoadBytes([]byte("billing:\n  mode: unknown\n")); err == nil {
		t.Fatal("unknown billing mode accepted")
	}
	if _, err := LoadBytes([]byte("billing:\n  mode: prepaid\n  currency: usd\n")); err == nil {
		t.Fatal("invalid billing currency accepted")
	}
}
