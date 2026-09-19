package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the startup configuration decoded from urbino.yaml.
type Config struct {
	HealthAddr  string `yaml:"health_addr" json:"health_addr"`
	Environment string `yaml:"environment" json:"environment"`
	LogLevel    string `yaml:"log_level" json:"log_level"`
	Development bool   `yaml:"development" json:"development"`
}

// AllowedEnvironmentNames is the complete application-owned environment map.
var AllowedEnvironmentNames = map[string]struct{}{
	EnvConfigPath:  {},
	EnvHealthAddr:  {},
	EnvLogLevel:    {},
	EnvEnvironment: {},
}

// Load decodes the configuration file at path. Malformed YAML and unknown
// fields are errors. An empty file is a valid configuration and yields zero
// values. Production configuration cannot enable development behavior.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: open %q: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("config: decode %q: %w", path, err)
	}

	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("config: decode %q: multiple YAML documents", path)
		}
		return Config{}, fmt.Errorf("config: decode %q: trailing YAML document: %w", path, err)
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Environment), "production") && cfg.Development {
		return Config{}, fmt.Errorf("config: development mode is forbidden in production")
	}
	return cfg, nil
}

// ApplyEnvironment applies only explicitly declared URBINO_ variables.
func ApplyEnvironment(cfg Config, values map[string]string) (Config, error) {
	for name := range values {
		if _, ok := AllowedEnvironmentNames[name]; !ok {
			return Config{}, fmt.Errorf("config: unsupported environment variable %q", name)
		}
	}
	if value := strings.TrimSpace(values[EnvHealthAddr]); value != "" {
		cfg.HealthAddr = value
	}
	if value := strings.TrimSpace(values[EnvLogLevel]); value != "" {
		cfg.LogLevel = value
	}
	if value := strings.TrimSpace(values[EnvEnvironment]); value != "" {
		cfg.Environment = value
	}
	if strings.EqualFold(cfg.Environment, "production") && cfg.Development {
		return Config{}, fmt.Errorf("config: development mode is forbidden in production")
	}
	return cfg, nil
}

// StrictJSONDecode rejects duplicate keys and excessive nesting before decoding.
func StrictJSONDecode(data []byte, target any) error {
	if err := validateJSON(bytes.NewReader(data), 32); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("config: decode JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("config: multiple JSON values")
		}
		return fmt.Errorf("config: trailing JSON value: %w", err)
	}
	return nil
}

// FieldConflict names two top-level JSON fields that cannot appear together.
type FieldConflict struct {
	Left  string
	Right string
}

// StrictJSONDecodeWithConflicts adds explicit semantic conflict checks to the
// duplicate-key, depth and unknown-field checks of StrictJSONDecode.
func StrictJSONDecodeWithConflicts(data []byte, target any, conflicts ...FieldConflict) error {
	if err := validateJSON(bytes.NewReader(data), 32); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("config: decode JSON fields: %w", err)
	}
	for _, conflict := range conflicts {
		_, left := fields[conflict.Left]
		_, right := fields[conflict.Right]
		if left && right {
			return fmt.Errorf("config: conflicting JSON fields %q and %q", conflict.Left, conflict.Right)
		}
	}
	return StrictJSONDecode(data, target)
}

// LoadWithEnvironment loads one selected file and applies only the explicit
// application environment mapping. The path selector itself remains separate.
func LoadWithEnvironment(path string, values map[string]string) (Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return Config{}, err
	}
	return ApplyEnvironment(cfg, values)
}
func validateJSON(r io.Reader, maxDepth int) error {
	dec := json.NewDecoder(r)
	if err := validateJSONValue(dec, 0, maxDepth); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("config: multiple JSON values")
		}
		return fmt.Errorf("config: trailing JSON value: %w", err)
	}
	return nil
}

func validateJSONValue(dec *json.Decoder, depth, maxDepth int) error {
	if depth > maxDepth {
		return fmt.Errorf("config: JSON nesting exceeds %d", maxDepth)
	}
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("config: invalid JSON: %w", err)
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			seen := make(map[string]struct{})
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return fmt.Errorf("config: invalid JSON object: %w", err)
				}
				key := keyToken.(string)
				normalizedKey := strings.ToLower(key)
				if _, duplicate := seen[normalizedKey]; duplicate {
					return fmt.Errorf("config: duplicate or conflicting JSON key %q", key)
				}
				seen[normalizedKey] = struct{}{}
				if err := validateJSONValue(dec, depth+1, maxDepth); err != nil {
					return err
				}
			}
			_, err = dec.Token()
		case '[':
			for dec.More() {
				if err := validateJSONValue(dec, depth+1, maxDepth); err != nil {
					return err
				}
			}
			_, err = dec.Token()
		}
	}
	if err != nil {
		return fmt.Errorf("config: invalid JSON container: %w", err)
	}
	return nil
}

// ResolveHealthAddr applies the documented precedence for the internal health
// listener address: URBINO_HEALTH_ADDR, then the configuration file, then
// DefaultHealthAddr.
func ResolveHealthAddr(cfg Config, envValue string) string {
	if v := strings.TrimSpace(envValue); v != "" {
		return v
	}
	if v := strings.TrimSpace(cfg.HealthAddr); v != "" {
		return v
	}
	return DefaultHealthAddr
}
