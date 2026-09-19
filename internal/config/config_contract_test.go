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
