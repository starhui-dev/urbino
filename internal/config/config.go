package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the startup configuration decoded from urbino.yaml.
type Config struct {
	// HealthAddr is the "host:port" of the internal health listener. Empty
	// means DefaultHealthAddr.
	HealthAddr string `yaml:"health_addr"`
}

// Load decodes the configuration file at path. Malformed YAML and unknown
// fields are errors: a mistyped setting must not be silently ignored. An empty
// file is a valid configuration and yields zero values.
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
	return cfg, nil
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
