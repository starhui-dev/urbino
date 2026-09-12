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
}

func TestEnvironmentContract(t *testing.T) {
	if err := ValidateEnvironment([]string{"URBINO_CONFIG=x", "URBINO_HEALTH_ADDR=:1"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEnvironment([]string{"URBINO_UNKNOWN=x"}); err == nil {
		t.Fatal("unknown env accepted")
	}
}
