package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"
)

// Config is the startup configuration decoded from urbino.yaml.
type Config struct {
	HealthAddr  string         `yaml:"health_addr" json:"health_addr"`
	Environment string         `yaml:"environment" json:"environment"`
	LogLevel    string         `yaml:"log_level" json:"log_level"`
	Development bool           `yaml:"development" json:"development"`
	Database    DatabaseConfig `yaml:"database" json:"database"`
}

// DatabaseConfig contains only a path to a restricted DSN file. The DSN is
// never stored in the startup YAML or emitted in an error.
type DatabaseConfig struct {
	DSNFile string `yaml:"dsn_file" json:"dsn_file"`
}

// AllowedEnvironmentNames is the complete application-owned environment map.
var AllowedEnvironmentNames = map[string]struct{}{
	EnvConfigPath:      {},
	EnvHealthAddr:      {},
	EnvLogLevel:        {},
	EnvEnvironment:     {},
	EnvDatabaseDSNFile: {},
}

// Load decodes the configuration file at path. Malformed YAML and unknown
// fields are errors. An empty file is a valid configuration and yields zero
// values; non-empty values are validated against the startup schema.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: open %q: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Config{}, nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return Config{}, fmt.Errorf("config: decode %q: %w", path, err)
	}
	if err := validateConfigYAMLNode(&document); err != nil {
		return Config{}, fmt.Errorf("config: validate %q: %w", path, err)
	}
	if len(document.Content) == 0 {
		return Config{}, nil
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: decode %q: %w", path, err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("config: decode %q: multiple YAML documents", path)
		}
		return Config{}, fmt.Errorf("config: decode %q: trailing YAML document: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config: validate %q: %w", path, err)
	}
	return cfg, nil
}

func validateConfigYAMLNode(document *yaml.Node) error {
	if len(document.Content) == 0 {
		return nil
	}
	if document.Kind != yaml.DocumentNode {
		return fmt.Errorf("configuration must be a YAML document")
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("configuration must be a YAML mapping")
	}
	mapping := document.Content[0]
	for i := 0; i < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i], mapping.Content[i+1]
		switch key.Value {
		case "health_addr", "environment", "log_level":
			if value.Tag != "!!str" || value.Value == "" {
				return fmt.Errorf("%s must be a non-empty string", key.Value)
			}
		case "development":
			if value.Tag != "!!bool" {
				return fmt.Errorf("development must be a boolean")
			}
		case "database":
			if value.Kind != yaml.MappingNode {
				return fmt.Errorf("database must be a mapping")
			}
			for j := 0; j < len(value.Content); j += 2 {
				dbKey, dbValue := value.Content[j], value.Content[j+1]
				if dbKey.Value == "dsn_file" && (dbValue.Tag != "!!str" || strings.TrimSpace(dbValue.Value) == "") {
					return fmt.Errorf("database.dsn_file must be a non-empty string")
				}
			}
		}
	}
	return nil
}

// ApplyEnvironment applies only explicitly declared URBINO_ variables and
// validates the resulting configuration before returning it.
func ApplyEnvironment(cfg Config, values map[string]string) (Config, error) {
	for name := range values {
		if _, ok := AllowedEnvironmentNames[name]; !ok {
			return Config{}, fmt.Errorf("config: unsupported environment variable %q", name)
		}
	}
	if raw, ok := values[EnvHealthAddr]; ok {
		if raw == "" || strings.TrimSpace(raw) != raw {
			return Config{}, fmt.Errorf("config: %s must be non-empty and trimmed", EnvHealthAddr)
		}
		cfg.HealthAddr = raw
	}
	if raw, ok := values[EnvLogLevel]; ok {
		if raw == "" || strings.TrimSpace(raw) != raw {
			return Config{}, fmt.Errorf("config: %s must be non-empty and trimmed", EnvLogLevel)
		}
		cfg.LogLevel = raw
	}
	if raw, ok := values[EnvEnvironment]; ok {
		if raw == "" || strings.TrimSpace(raw) != raw {
			return Config{}, fmt.Errorf("config: %s must be non-empty and trimmed", EnvEnvironment)
		}
		cfg.Environment = raw
	}
	if raw, ok := values[EnvDatabaseDSNFile]; ok {
		if raw == "" || strings.TrimSpace(raw) != raw {
			return Config{}, fmt.Errorf("config: %s must be non-empty and trimmed", EnvDatabaseDSNFile)
		}
		cfg.Database.DSNFile = raw
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate enforces the same semantic constraints as api/config.schema.json.
// Empty optional fields remain valid so a minimal configuration can use
// runtime defaults; once set, values must be from the closed startup schema.
func (cfg Config) Validate() error {
	if cfg.HealthAddr != "" && (strings.TrimSpace(cfg.HealthAddr) != cfg.HealthAddr || strings.TrimSpace(cfg.HealthAddr) == "") {
		return fmt.Errorf("config: health_addr must be non-empty and trimmed")
	}
	if cfg.Environment != "" {
		if strings.TrimSpace(cfg.Environment) != cfg.Environment {
			return fmt.Errorf("config: environment must be trimmed")
		}
		switch cfg.Environment {
		case "development", "test", "production":
		default:
			return fmt.Errorf("config: invalid environment %q", cfg.Environment)
		}
	}
	if cfg.LogLevel != "" {
		if strings.TrimSpace(cfg.LogLevel) != cfg.LogLevel {
			return fmt.Errorf("config: log_level must be trimmed")
		}
		switch cfg.LogLevel {
		case "debug", "info", "warn", "error":
		default:
			return fmt.Errorf("config: invalid log_level %q", cfg.LogLevel)
		}
	}
	if cfg.Database.DSNFile != "" && strings.TrimSpace(cfg.Database.DSNFile) != cfg.Database.DSNFile {
		return fmt.Errorf("config: database.dsn_file must be trimmed")
	}
	if cfg.Environment == "production" && cfg.Development {
		return fmt.Errorf("config: development mode is forbidden in production")
	}
	return nil
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

// ReadDatabaseDSN reads the DSN from the configured restricted file without
// exposing its contents in returned errors. The file is opened without
// following symbolic links and inspected through the same descriptor that is
// read, so a path swap between the checks and the read cannot bypass them. The
// descriptor must be a regular file owned by the current effective user with
// no group or other permission bits set.
func ReadDatabaseDSN(cfg Config) (string, error) {
	path := strings.TrimSpace(cfg.Database.DSNFile)
	if path == "" {
		return "", fmt.Errorf("config: database.dsn_file is required")
	}
	// O_NOFOLLOW refuses a symlink at the final path component. O_NONBLOCK
	// keeps the open from stalling on a FIFO or device node, which the fstat
	// checks below then reject as non-regular; it is ignored for regular files.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return "", fmt.Errorf("config: database DSN file must not be a symbolic link")
		}
		return "", fmt.Errorf("config: database DSN file is unavailable")
	}
	file := os.NewFile(uintptr(fd), "database.dsn_file")
	if file == nil {
		_ = syscall.Close(fd)
		return "", fmt.Errorf("config: database DSN file is unavailable")
	}
	defer func() { _ = file.Close() }()

	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return "", fmt.Errorf("config: database DSN file is unavailable")
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		return "", fmt.Errorf("config: database DSN file must be a regular file")
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return "", fmt.Errorf("config: database DSN file must be owned by the current user")
	}
	if stat.Mode&0o077 != 0 {
		return "", fmt.Errorf("config: database DSN file must not be accessible by group or others")
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("config: database DSN file is unreadable")
	}
	dsn := strings.TrimSpace(string(data))
	if dsn == "" {
		return "", fmt.Errorf("config: database DSN file is empty")
	}
	return dsn, nil
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
