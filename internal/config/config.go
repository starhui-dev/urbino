package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const MaxInputBytes = 1 << 20

// Config 是启动所需的强类型配置。未实现的业务能力保持关闭。
type Config struct {
	Environment   string        `yaml:"environment"`
	Server        Server        `yaml:"server"`
	Database      Database      `yaml:"database"`
	Valkey        Valkey        `yaml:"valkey"`
	Secrets       Secrets       `yaml:"secrets"`
	Transport     Transport     `yaml:"transport"`
	Security      Security      `yaml:"security"`
	Limits        Limits        `yaml:"limits"`
	Billing       Billing       `yaml:"billing"`
	Jobs          Jobs          `yaml:"jobs"`
	Observability Observability `yaml:"observability"`
	Retention     Retention     `yaml:"retention"`
}

type Server struct {
	Public   Listener `yaml:"public"`
	Admin    Listener `yaml:"admin"`
	Internal Listener `yaml:"internal"`
}
type Listener struct {
	Listen  string `yaml:"listen"`
	Enabled bool   `yaml:"enabled"`
}
type Database struct {
	URLFile string `yaml:"url_file"`
	URL     string `yaml:"url"`
}
type Valkey struct {
	URLFile string `yaml:"url_file"`
	URL     string `yaml:"url"`
}
type Secrets struct {
	KMSRef    string `yaml:"kms_ref"`
	Directory string `yaml:"directory"`
}
type Transport struct {
	ProviderAllowlist []string `yaml:"provider_allowlist"`
	TLSVerify         bool     `yaml:"tls_verify"`
	ProxyURL          string   `yaml:"proxy_url"`
}
type Security struct {
	TestMode    bool `yaml:"test_mode"`
	Pprof       bool `yaml:"pprof"`
	AdminPublic bool `yaml:"admin_public"`
}
type Limits struct {
	MaxBodyBytes   int64 `yaml:"max_body_bytes"`
	MaxStreamBytes int64 `yaml:"max_stream_bytes"`
	MaxConcurrency int   `yaml:"max_concurrency"`
}
type Billing struct {
	Mode     string `yaml:"mode"`
	Currency string `yaml:"currency"`
}
type Jobs struct {
	Enabled      bool   `yaml:"enabled"`
	DrainTimeout string `yaml:"drain_timeout"`
}
type Observability struct {
	LogLevel    string `yaml:"log_level"`
	ServiceName string `yaml:"service_name"`
}
type Retention struct {
	RequestDays int `yaml:"request_days"`
	UsageDays   int `yaml:"usage_days"`
}

// Validate checks cross-field safety constraints after decoding.
func (c Config) Validate() error { return validate(c) }

func Default() Config {
	return Config{Environment: "development", Server: Server{Internal: Listener{Listen: "127.0.0.1:9091", Enabled: true}}, Transport: Transport{TLSVerify: true}, Billing: Billing{Mode: "disabled", Currency: ""}, Observability: Observability{ServiceName: "urbino", LogLevel: "info"}}
}

// LoadPath 严格加载单文档 YAML；explicit 只用于错误消息，不会静默回退。
func LoadPath(path string, explicit bool) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if explicit {
			return Config{}, fmt.Errorf("config %q: %w", path, err)
		}
		return Config{}, err
	}
	c, err := LoadBytes(b)
	if err != nil {
		return Config{}, fmt.Errorf("config %q: %w", path, err)
	}
	return c, nil
}

func LoadBytes(data []byte) (Config, error) {
	if len(data) > MaxInputBytes {
		return Config{}, fmt.Errorf("config exceeds %d bytes", MaxInputBytes)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Default(), nil
	}
	var root yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(&root); err != nil {
		return Config{}, fmt.Errorf("invalid YAML: %w", err)
	}
	if err := validateNode(&root); err != nil {
		return Config{}, err
	}
	var extra yaml.Node
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, errors.New("multiple YAML documents are not allowed")
		}
		return Config{}, fmt.Errorf("invalid YAML document: %w", err)
	}
	c := Default()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("config fields: %w", err)
	}
	if err := validate(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func validateNode(n *yaml.Node) error {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.AliasNode {
		return errors.New("YAML aliases are not allowed")
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Kind != yaml.ScalarNode {
				return errors.New("mapping keys must be scalars")
			}
			if seen[k.Value] {
				return fmt.Errorf("duplicate YAML key %q", k.Value)
			}
			seen[k.Value] = true
			if err := validateNode(v); err != nil {
				return err
			}
		}
	} else {
		for _, c := range n.Content {
			if err := validateNode(c); err != nil {
				return err
			}
		}
	}
	return nil
}

func validate(c Config) error {
	if c.Environment == "" {
		c.Environment = "development"
	}
	if c.Environment != "development" && c.Environment != "production" {
		return fmt.Errorf("environment must be development or production")
	}
	if c.Environment == "production" && c.Security.TestMode {
		return errors.New("test_mode is forbidden in production")
	}
	if c.Environment == "production" && c.Security.AdminPublic {
		return errors.New("admin_public is forbidden in production")
	}
	if c.Environment == "production" && isPublicListen(c.Server.Admin.Listen) {
		return errors.New("admin listener must not be public in production")
	}
	if c.Environment == "production" && !c.Transport.TLSVerify {
		return errors.New("tls_verify must be enabled in production")
	}
	if c.Environment == "production" && (c.Database.URL != "" || c.Valkey.URL != "") {
		return errors.New("production database and valkey URLs must use *_file references")
	}
	if c.Limits.MaxBodyBytes < 0 || c.Limits.MaxStreamBytes < 0 || c.Limits.MaxConcurrency < 0 {
		return errors.New("limits must not be negative")
	}
	if c.Jobs.DrainTimeout != "" {
		d, err := time.ParseDuration(c.Jobs.DrainTimeout)
		if err != nil || d <= 0 || d > time.Hour {
			return errors.New("jobs.drain_timeout must be a duration between 1ns and 1h")
		}
	}
	if c.Transport.ProviderAllowlist == nil && c.Environment == "production" {
		return errors.New("provider_allowlist must be non-empty in production")
	}
	if c.Environment == "production" && len(c.Transport.ProviderAllowlist) == 0 {
		return errors.New("provider_allowlist must be non-empty in production")
	}
	return nil
}

func isPublicListen(addr string) bool {
	addr = strings.TrimSpace(addr)
	return strings.HasPrefix(addr, ":") || strings.HasPrefix(addr, "0.0.0.0:") || strings.HasPrefix(addr, "[::]:") || addr == "0.0.0.0" || addr == "::"
}

var allowedEnv = map[string]bool{"URBINO_CONFIG": true, "URBINO_HEALTH_ADDR": true}

func ValidateEnvironment(environ []string) error {
	for _, item := range environ {
		name, _, ok := strings.Cut(item, "=")
		if ok && strings.HasPrefix(name, "URBINO_") && !allowedEnv[name] {
			return fmt.Errorf("unknown environment variable %s", name)
		}
	}
	return nil
}

func ResolvePath(flagPath string, environ []string, cwd string) (path string, explicit bool, err error) {
	if err = ValidateEnvironment(environ); err != nil {
		return "", false, err
	}
	if flagPath != "" {
		return flagPath, true, nil
	}
	for _, item := range environ {
		name, value, _ := strings.Cut(item, "=")
		if name == "URBINO_CONFIG" && value != "" {
			return value, true, nil
		}
	}
	return filepath.Join(cwd, "urbino.yaml"), false, nil
}
