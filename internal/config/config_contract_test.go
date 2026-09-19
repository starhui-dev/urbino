package config

import (
	"testing"
)

type jsonConfig struct {
	Name string `json:"name"`
}

type conflictConfig struct {
	Input    string `json:"input"`
	Messages string `json:"messages"`
}

func TestStrictJSONDecodeRejectsConflictingFields(t *testing.T) {
	var cfg conflictConfig
	err := StrictJSONDecodeWithConflicts(
		[]byte(`{"input":"a","messages":"b"}`),
		&cfg,
		FieldConflict{Left: "input", Right: "messages"},
	)
	if err == nil {
		t.Fatal("conflicting JSON fields accepted")
	}
}

func TestStrictJSONDecodeRejectsAmbiguityAndDepth(t *testing.T) {
	var cfg jsonConfig
	if err := StrictJSONDecode([]byte(`{"name":"a","name":"b"}`), &cfg); err == nil {
		t.Fatal("duplicate JSON key accepted")
	}
	deep := "{}"
	for range 40 {
		deep = `{"next":` + deep + `}`
	}
	if err := StrictJSONDecode([]byte(deep), &cfg); err == nil {
		t.Fatal("overly deep JSON accepted")
	}
}

func TestApplyEnvironmentRejectsUnknownAndProductionDevelopment(t *testing.T) {
	if _, err := ApplyEnvironment(Config{}, map[string]string{"URBINO_UNKNOWN": "x"}); err == nil {
		t.Fatal("unknown environment variable accepted")
	}
	if _, err := ApplyEnvironment(Config{Development: true}, map[string]string{EnvEnvironment: "production"}); err == nil {
		t.Fatal("production development mode accepted")
	}
}

func TestConfigValidationRejectsInvalidSchemaValues(t *testing.T) {
	cases := map[string]string{
		EnvEnvironment: "staging",
		EnvLogLevel:    "trace",
	}
	for key, value := range cases {
		if _, err := ApplyEnvironment(Config{}, map[string]string{key: value}); err == nil {
			t.Fatalf("invalid %s accepted", key)
		}
	}
	for _, cfg := range []Config{{HealthAddr: "   "}, {Environment: " production "}, {LogLevel: " info "}} {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("non-canonical config accepted: %+v", cfg)
		}
	}
	if _, err := ApplyEnvironment(Config{Environment: "test"}, map[string]string{EnvEnvironment: "   "}); err == nil {
		t.Fatal("blank environment override accepted")
	}
}

func TestLoadRejectsExplicitNullAndWrongTypes(t *testing.T) {
	for _, content := range []string{"health_addr: null\n", "environment: null\n", "log_level: null\n", "development: null\n", "development: yes\n"} {
		if _, err := Load(writeFile(t, t.TempDir(), "config.yaml", content)); err == nil {
			t.Fatalf("invalid YAML type accepted: %q", content)
		}
	}
}
