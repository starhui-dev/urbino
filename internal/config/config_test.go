package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestResolvePathPrecedence(t *testing.T) {
	dir := t.TempDir()
	flagFile := writeFile(t, dir, "flag.yaml", "")
	envFile := writeFile(t, dir, "env.yaml", "")
	defaultFile := writeFile(t, dir, DefaultFileName, "")

	t.Run("flag beats env and default", func(t *testing.T) {
		got, err := ResolvePath(PathInput{Explicit: flagFile, Env: envFile, Dir: dir})
		if err != nil {
			t.Fatalf("ResolvePath: %v", err)
		}
		if got.File != flagFile || got.Source != SourceFlag {
			t.Fatalf("got %+v, want %s from %s", got, flagFile, SourceFlag)
		}
	})

	t.Run("env beats default", func(t *testing.T) {
		got, err := ResolvePath(PathInput{Env: envFile, Dir: dir})
		if err != nil {
			t.Fatalf("ResolvePath: %v", err)
		}
		if got.File != envFile || got.Source != SourceEnv {
			t.Fatalf("got %+v, want %s from %s", got, envFile, SourceEnv)
		}
	})

	t.Run("default is urbino.yaml in the working directory", func(t *testing.T) {
		got, err := ResolvePath(PathInput{Dir: dir})
		if err != nil {
			t.Fatalf("ResolvePath: %v", err)
		}
		if got.File != defaultFile || got.Source != SourceDefault {
			t.Fatalf("got %+v, want %s from %s", got, defaultFile, SourceDefault)
		}
	})

	t.Run("blank flag and env fall through", func(t *testing.T) {
		got, err := ResolvePath(PathInput{Explicit: "  ", Env: "", Dir: dir})
		if err != nil {
			t.Fatalf("ResolvePath: %v", err)
		}
		if got.File != defaultFile {
			t.Fatalf("got %q, want %q", got.File, defaultFile)
		}
	})
}

func TestResolvePathMissingFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	envFile := writeFile(t, dir, "env.yaml", "")
	missing := filepath.Join(dir, "missing.yaml")

	t.Run("explicit missing file does not fall back", func(t *testing.T) {
		_, err := ResolvePath(PathInput{Explicit: missing, Env: envFile, Dir: dir})
		if !errors.Is(err, ErrNotExist) {
			t.Fatalf("err = %v, want ErrNotExist", err)
		}
	})

	t.Run("env missing file does not fall back", func(t *testing.T) {
		_, err := ResolvePath(PathInput{Env: missing, Dir: dir})
		if !errors.Is(err, ErrNotExist) {
			t.Fatalf("err = %v, want ErrNotExist", err)
		}
	})

	t.Run("missing default file", func(t *testing.T) {
		_, err := ResolvePath(PathInput{Dir: dir})
		if !errors.Is(err, ErrNotExist) {
			t.Fatalf("err = %v, want ErrNotExist", err)
		}
	})

	t.Run("directory is not a configuration file", func(t *testing.T) {
		_, err := ResolvePath(PathInput{Explicit: dir})
		if !errors.Is(err, ErrNotRegular) {
			t.Fatalf("err = %v, want ErrNotRegular", err)
		}
	})
}

func TestLoadDecodesHealthAddr(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "urbino.yaml", "health_addr: 127.0.0.1:18091\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HealthAddr != "127.0.0.1:18091" {
		t.Fatalf("HealthAddr = %q", cfg.HealthAddr)
	}
}

func TestLoadSupportsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "urbino.yaml", "# no settings yet\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HealthAddr != "" {
		t.Fatalf("HealthAddr = %q, want empty", cfg.HealthAddr)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	dir := t.TempDir()

	t.Run("unknown field", func(t *testing.T) {
		path := writeFile(t, dir, "unknown.yaml", "healt_addr: 127.0.0.1:1\n")
		if _, err := Load(path); err == nil {
			t.Fatal("Load accepted an unknown field")
		}
	})
	t.Run("multiple documents", func(t *testing.T) {
		path := writeFile(t, dir, "multiple.yaml", "health_addr: 127.0.0.1:1\n---\nunknown: true\n")
		if _, err := Load(path); err == nil {
			t.Fatal("Load accepted multiple YAML documents")
		}
	})

	t.Run("malformed yaml", func(t *testing.T) {
		path := writeFile(t, dir, "broken.yaml", "health_addr: [unterminated\n")
		if _, err := Load(path); err == nil {
			t.Fatal("Load accepted malformed YAML")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if _, err := Load(filepath.Join(dir, "absent.yaml")); err == nil {
			t.Fatal("Load accepted a missing file")
		}
	})
}

func TestResolveHealthAddrPrecedence(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		env  string
		want string
	}{
		{name: "default", want: DefaultHealthAddr},
		{name: "config", cfg: Config{HealthAddr: "127.0.0.1:18091"}, want: "127.0.0.1:18091"},
		{name: "env beats config", cfg: Config{HealthAddr: "127.0.0.1:18091"}, env: "127.0.0.1:28091", want: "127.0.0.1:28091"},
		{name: "blank env falls back to config", cfg: Config{HealthAddr: "127.0.0.1:18091"}, env: " ", want: "127.0.0.1:18091"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveHealthAddr(tc.cfg, tc.env); got != tc.want {
				t.Fatalf("ResolveHealthAddr = %q, want %q", got, tc.want)
			}
		})
	}
}
