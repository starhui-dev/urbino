package contracts_test

// P01-T04 (checklists/test-matrix.csv: 未知配置与 production 开发开关):
// unknown configuration fields are rejected, development switches cannot
// enter production, and only the four explicitly declared URBINO_ variables
// are honored (--config > URBINO_CONFIG > ./urbino.yaml path selection itself
// is covered by the stage 00 foundation tests).

import (
	"path/filepath"
	"testing"

	"example.com/urbino/internal/config"
)

func TestP01T04UnknownConfigFieldsRejected(t *testing.T) {
	for name, content := range map[string]string{
		"undeclared key": "health_addr: 127.0.0.1:9101\ntotally_unknown_setting: true\n",
		"misspelled key": "health_adr: 127.0.0.1:9101\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := writeConfigFile(t, content)
			if _, err := config.Load(path); err == nil {
				t.Fatalf("config.Load accepted an unknown field:\n%s", content)
			}
		})
	}
}

func TestP01T04ProductionDevelopmentSwitchRejected(t *testing.T) {
	t.Run("production forbids the development switch", func(t *testing.T) {
		path := writeConfigFile(t, "environment: production\ndevelopment: true\n")
		if _, err := config.Load(path); err == nil {
			t.Fatal("production configuration must not enable development mode")
		}
	})
	t.Run("production forbids undeclared test switches", func(t *testing.T) {
		path := writeConfigFile(t, "environment: production\ndev_mode: true\n")
		if _, err := config.Load(path); err == nil {
			t.Fatal("production configuration must reject the undeclared dev_mode switch")
		}
	})
	t.Run("development may enable the switch", func(t *testing.T) {
		path := writeConfigFile(t, "environment: development\ndevelopment: true\n")
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatalf("development configuration with the switch must load: %v", err)
		}
		if !cfg.Development {
			t.Fatal("development switch must be preserved outside production")
		}
	})
	t.Run("production without switches loads", func(t *testing.T) {
		path := writeConfigFile(t, "environment: production\nhealth_addr: 127.0.0.1:9102\nlog_level: info\n")
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatalf("clean production configuration must load: %v", err)
		}
		if cfg.Environment != "production" {
			t.Fatalf("environment = %q, want production", cfg.Environment)
		}
	})
}

func TestP01T04ShippedTemplatesMatchContract(t *testing.T) {
	root := findRepoRoot(t)
	prod, err := config.Load(filepath.Join(root, "configs", "urbino.production.example.yaml"))
	if err != nil {
		t.Fatalf("shipped production template must load: %v", err)
	}
	if prod.Environment != "production" || prod.Development {
		t.Fatalf("shipped production template = %+v, want production without development switch", prod)
	}
	dev, err := config.Load(filepath.Join(root, "configs", "urbino.example.yaml"))
	if err != nil {
		t.Fatalf("shipped development template must load: %v", err)
	}
	if dev.Environment != "development" {
		t.Fatalf("shipped development template environment = %q, want development", dev.Environment)
	}
}

func TestP01T04ExplicitEnvironmentAllowlist(t *testing.T) {
	base := config.Config{HealthAddr: "127.0.0.1:9102", Environment: "development"}

	t.Run("declared variables are applied", func(t *testing.T) {
		cfg, err := config.ApplyEnvironment(base, map[string]string{
			"URBINO_HEALTH_ADDR": "127.0.0.1:9103",
			"URBINO_LOG_LEVEL":   "debug",
			"URBINO_ENV":         "production",
			"URBINO_CONFIG":      "/tmp/unused-by-config.yaml",
		})
		if err != nil {
			t.Fatalf("declared URBINO_ variables must be accepted: %v", err)
		}
		if cfg.HealthAddr != "127.0.0.1:9103" {
			t.Errorf("URBINO_HEALTH_ADDR not applied: %q", cfg.HealthAddr)
		}
		if cfg.LogLevel != "debug" {
			t.Errorf("URBINO_LOG_LEVEL not applied: %q", cfg.LogLevel)
		}
		if cfg.Environment != "production" {
			t.Errorf("URBINO_ENV not applied: %q", cfg.Environment)
		}
	})

	t.Run("undeclared variables are rejected", func(t *testing.T) {
		if _, err := config.ApplyEnvironment(base, map[string]string{"URBINO_TOTALLY_UNKNOWN": "1"}); err == nil {
			t.Fatal("undeclared URBINO_ variables must be rejected, not silently ignored")
		}
	})

	t.Run("environment beats file for the production switch", func(t *testing.T) {
		path := writeConfigFile(t, "environment: development\ndevelopment: true\n")
		if _, err := config.Load(path); err != nil {
			t.Fatalf("development file alone must load: %v", err)
		}
		if _, err := config.LoadWithEnvironment(path, map[string]string{"URBINO_ENV": "production"}); err == nil {
			t.Fatal("URBINO_ENV=production must forbid the development switch of the selected file")
		}
	})

	t.Run("environment beats file for the health address", func(t *testing.T) {
		path := writeConfigFile(t, "health_addr: 127.0.0.1:9102\n")
		cfg, err := config.LoadWithEnvironment(path, map[string]string{"URBINO_HEALTH_ADDR": "127.0.0.1:9103"})
		if err != nil {
			t.Fatalf("load with environment: %v", err)
		}
		if cfg.HealthAddr != "127.0.0.1:9103" {
			t.Fatalf("health addr = %q, want the URBINO_HEALTH_ADDR override", cfg.HealthAddr)
		}
	})
}
