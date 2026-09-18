// Package cli implements the urbino command line.
//
// Run is a pure function of its arguments, injected streams, environment and
// context, so every command can be exercised without spawning a process or
// touching the real environment.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"example.com/urbino/internal/config"
	"example.com/urbino/internal/httpapi"
	"example.com/urbino/internal/version"
)

// Process exit codes.
const (
	// ExitOK reports a successful command.
	ExitOK = 0
	// ExitError reports a runtime failure, such as a configuration or listener error.
	ExitError = 1
	// ExitUsage reports invalid command line input.
	ExitUsage = 2
)

const usage = `urbino - multi-model gateway

Usage:
  urbino [--config <file>] version
  urbino [--config <file>] serve
  urbino [--config <file>] help

Commands:
  version   print the version of this binary
  serve     run the internal health listener
  help      print this help text

Flags:
  --config <file>   configuration file; takes precedence over URBINO_CONFIG and ./urbino.yaml

Environment:
  URBINO_CONFIG      configuration file used when --config is absent
  URBINO_HEALTH_ADDR internal health listener address, e.g. 127.0.0.1:9091

Commands planned for later stages (worker, migrate, bootstrap, admin, doctor,
config validate) are not implemented yet and fail with a usage error.
`

// Options are the process inputs of Run.
type Options struct {
	// Args are the command line arguments without the program name.
	Args []string
	// Stdout receives command output.
	Stdout io.Writer
	// Stderr receives diagnostics.
	Stderr io.Writer
	// LookupEnv reads an environment variable; nil means no variables are set.
	LookupEnv func(string) (string, bool)
	// WorkDir is the directory searched for the default configuration file;
	// empty means the process working directory.
	WorkDir string
}

func (o *Options) env(name string) string {
	if o.LookupEnv == nil {
		return ""
	}
	v, _ := o.LookupEnv(name)
	return v
}

func (o *Options) fail(format string, args ...any) {
	fmt.Fprintf(o.Stderr, "urbino: "+format+"\n", args...)
}

func (o *Options) usageError(format string, args ...any) int {
	fmt.Fprintf(o.Stderr, "urbino: "+format+"\n", args...)
	fmt.Fprint(o.Stderr, usage)
	return ExitUsage
}

// invocation is the parsed command line.
type invocation struct {
	command    string
	args       []string
	configPath string
	help       bool
}

// Run executes one command and returns the process exit code.
func Run(ctx context.Context, o Options) int {
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}

	inv, err := parseArgs(o.Args)
	if err != nil {
		return o.usageError("%v", err)
	}
	if inv.help {
		switch inv.command {
		case "", "help", "version", "serve":
			fmt.Fprint(o.Stdout, usage)
			return ExitOK
		default:
			return o.usageError("unknown command %q", inv.command)
		}
	}

	switch inv.command {
	case "":
		return o.usageError("no command given")
	case "help":
		if len(inv.args) > 0 {
			return o.usageError("help takes no arguments")
		}
		fmt.Fprint(o.Stdout, usage)
		return ExitOK
	case "version":
		if len(inv.args) > 0 {
			return o.usageError("version takes no arguments")
		}
		fmt.Fprintf(o.Stdout, "urbino version %s\n", version.String())
		return ExitOK
	case "serve":
		if len(inv.args) > 0 {
			return o.usageError("serve takes no arguments")
		}
		return o.serve(ctx, inv.configPath)
	default:
		return o.usageError("unknown command %q", inv.command)
	}
}

// parseArgs extracts the global --config flag wherever it appears, the command
// and the remaining command arguments.
func parseArgs(args []string) (invocation, error) {
	var inv invocation
	gaveConfig := false
	rest := args
	for len(rest) > 0 {
		a := rest[0]
		rest = rest[1:]
		switch {
		case a == "--config" || a == "-config":
			if len(rest) == 0 {
				return invocation{}, errors.New("--config requires a file path")
			}
			inv.configPath = rest[0]
			rest = rest[1:]
			gaveConfig = true
		case strings.HasPrefix(a, "--config="):
			inv.configPath = strings.TrimPrefix(a, "--config=")
			gaveConfig = true
		case strings.HasPrefix(a, "-config="):
			inv.configPath = strings.TrimPrefix(a, "-config=")
			gaveConfig = true
		case a == "--help" || a == "-h" || a == "-help":
			inv.help = true
		case strings.HasPrefix(a, "-") && a != "-":
			return invocation{}, fmt.Errorf("unknown flag %q", a)
		case inv.command == "":
			inv.command = a
		default:
			inv.args = append(inv.args, a)
		}
	}
	if gaveConfig && strings.TrimSpace(inv.configPath) == "" {
		return invocation{}, errors.New("--config requires a file path")
	}
	return inv, nil
}

// serve loads the selected configuration and runs the internal health listener
// until ctx is done.
func (o *Options) serve(ctx context.Context, configFile string) int {
	path, err := config.ResolvePath(config.PathInput{
		Explicit: configFile,
		Env:      o.env(config.EnvConfigPath),
		Dir:      o.WorkDir,
	})
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}

	cfg, err := config.Load(path.File)
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}

	addr := config.ResolveHealthAddr(cfg, o.env(config.EnvHealthAddr))
	srv, err := httpapi.NewHealthServer(addr)
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}
	if err := srv.Listen(); err != nil {
		o.fail("%v", err)
		return ExitError
	}

	fmt.Fprintf(o.Stderr, "urbino: config %s (source %s)\n", path.File, path.Source)
	fmt.Fprintf(o.Stderr, "urbino: internal health listener on %s, GET %s only\n", srv.Addr(), httpapi.HealthPath)

	if err := srv.Serve(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ExitOK
		}
		o.fail("%v", err)
		return ExitError
	}
	fmt.Fprintf(o.Stderr, "urbino: health listener stopped\n")
	return ExitOK
}
