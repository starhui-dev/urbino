// Package config loads the Urbino startup configuration file.
//
// The startup file is urbino.yaml: it carries only process-level settings such
// as the internal health listener address. Tenant, provider, routing, quota and
// credential data live in PostgreSQL and are introduced by later stages.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// EnvConfigPath selects the configuration file when --config is absent.
	EnvConfigPath = "URBINO_CONFIG"

	// EnvHealthAddr overrides the health listener address.
	EnvHealthAddr = "URBINO_HEALTH_ADDR"

	// DefaultFileName is looked up in the working directory when neither
	// --config nor URBINO_CONFIG is set.
	DefaultFileName = "urbino.yaml"

	// DefaultHealthAddr is the standard internal health listener address. It is
	// a loopback address: the health listener never binds every interface
	// unless an operator overrides it explicitly.
	DefaultHealthAddr = "127.0.0.1:9091"
)

// Source identifies the rule that selected a configuration file.
type Source string

// Configuration file sources, highest precedence first.
const (
	SourceFlag    Source = "--config"
	SourceEnv     Source = EnvConfigPath
	SourceDefault Source = DefaultFileName
)

// Path is the single configuration file selected for one run.
type Path struct {
	// File is the selected file, as given by the operator or the default name.
	File string
	// Source is the rule that selected it.
	Source Source
}

// PathInput carries the raw inputs of path resolution.
type PathInput struct {
	// Explicit is the --config value; empty when the flag was not given.
	Explicit string
	// Env is the URBINO_CONFIG value; empty when the variable is unset.
	Env string
	// Dir is the working directory used to locate DefaultFileName; empty means
	// the process working directory.
	Dir string
}

var (
	// ErrNotExist reports that the selected configuration file is missing.
	ErrNotExist = errors.New("configuration file does not exist")

	// ErrNotRegular reports that the selected configuration path is not a
	// regular file.
	ErrNotRegular = errors.New("configuration path is not a regular file")
)

// ResolvePath selects exactly one configuration file following the documented
// precedence --config > URBINO_CONFIG > ./urbino.yaml. Sources are never
// merged, and the selected file must exist: a missing --config or URBINO_CONFIG
// target is an error rather than a silent fall back to the next rule.
func ResolvePath(in PathInput) (Path, error) {
	if v := strings.TrimSpace(in.Explicit); v != "" {
		return selectFile(Path{File: v, Source: SourceFlag})
	}
	if v := strings.TrimSpace(in.Env); v != "" {
		return selectFile(Path{File: v, Source: SourceEnv})
	}
	dir := strings.TrimSpace(in.Dir)
	if dir == "" {
		dir = "."
	}
	return selectFile(Path{File: filepath.Join(dir, DefaultFileName), Source: SourceDefault})
}

func selectFile(p Path) (Path, error) {
	info, err := os.Stat(p.File)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Path{}, fmt.Errorf("config: %s: %q: %w", p.Source, p.File, ErrNotExist)
	case err != nil:
		return Path{}, fmt.Errorf("config: %s: %q: %w", p.Source, p.File, err)
	case !info.Mode().IsRegular():
		return Path{}, fmt.Errorf("config: %s: %q: %w", p.Source, p.File, ErrNotRegular)
	}
	return p, nil
}
