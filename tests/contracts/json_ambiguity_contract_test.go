package contracts_test

// P01-T05 (checklists/test-matrix.csv: JSON 歧义): the strict JSON decoder
// must reject duplicate keys, conflicting field spellings, excessive nesting
// and multiple JSON documents instead of silently picking one interpretation.

import (
	"encoding/json"
	"strings"
	"testing"

	"example.com/urbino/internal/config"
)

func decodeConfigJSON(t *testing.T, raw string) (config.Config, error) {
	t.Helper()
	var cfg config.Config
	err := config.StrictJSONDecode([]byte(raw), &cfg)
	return cfg, err
}

func TestP01T05StrictJSONAcceptsWellFormedConfig(t *testing.T) {
	cfg, err := decodeConfigJSON(t, `{"health_addr":"127.0.0.1:9201","environment":"development","log_level":"info"}`)
	if err != nil {
		t.Fatalf("well-formed JSON config must decode: %v", err)
	}
	if cfg.HealthAddr != "127.0.0.1:9201" || cfg.Environment != "development" || cfg.LogLevel != "info" {
		t.Fatalf("decoded config = %+v", cfg)
	}
}

func TestP01T05StrictJSONRejectsUnknownAndDuplicateKeys(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown field":       `{"no_such_field":1}`,
		"exact duplicate key": `{"environment":"development","environment":"production"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeConfigJSON(t, raw); err == nil {
				t.Fatalf("StrictJSONDecode accepted: %s", raw)
			}
		})
	}
}

func TestP01T05StrictJSONRejectsConflictingFields(t *testing.T) {
	// "Health_Addr" and "health_addr" both select the same configuration
	// field through case-insensitive matching. A strict decoder must refuse
	// to choose one of them silently.
	raw := `{"health_addr":"127.0.0.1:9202","Health_Addr":"127.0.0.1:9203"}`
	cfg, err := decodeConfigJSON(t, raw)
	if err == nil {
		t.Fatalf("conflicting field spellings must be rejected, got %+v", cfg)
	}
}

func TestP01T05StrictJSONRejectsExcessiveNesting(t *testing.T) {
	var v any
	tooDeep := strings.Repeat("[", 40) + strings.Repeat("]", 40)
	if err := config.StrictJSONDecode([]byte(tooDeep), &v); err == nil {
		t.Fatal("JSON nesting beyond the documented limit must be rejected")
	}
	absurd := strings.Repeat("[", 100_000) + strings.Repeat("]", 100_000)
	if err := config.StrictJSONDecode([]byte(absurd), &v); err == nil {
		t.Fatal("absurdly deep JSON must be rejected without exhausting the stack")
	}
	shallow := strings.Repeat("[", 3) + strings.Repeat("]", 3)
	if err := config.StrictJSONDecode([]byte(shallow), &v); err != nil {
		t.Fatalf("bounded nesting must decode: %v", err)
	}
}

func TestP01T05StrictJSONRejectsMultipleAndMalformedDocuments(t *testing.T) {
	for name, raw := range map[string]string{
		"two JSON values": `{"health_addr":"a"} {"health_addr":"b"}`,
		"malformed JSON":  `{not json`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeConfigJSON(t, raw); err == nil {
				t.Fatalf("StrictJSONDecode accepted: %s", raw)
			}
		})
	}
}

// The envelope contract is also exercised at the JSON level: marshalling the
// public error must not lose or duplicate contractual keys (see
// TestP01T06PublicErrorEnvelopeJSON).
func TestP01T05StrictJSONConfigTargetIsTyped(t *testing.T) {
	raw := `{"health_addr":"127.0.0.1:9204"}`
	cfg, err := decodeConfigJSON(t, raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back["health_addr"] != "127.0.0.1:9204" {
		t.Fatalf("round trip lost health_addr: %s", out)
	}
}
