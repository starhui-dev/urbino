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
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/config"
	"example.com/urbino/internal/httpapi"
	"example.com/urbino/internal/storage/migrate"
	"example.com/urbino/internal/storage/postgres"
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
  urbino [--config <file>] migrate
  urbino [--config <file>] bootstrap --output <new-file> --pepper-file <restricted-file>
  urbino [--config <file>] help

Commands:
  version   print the version of this binary
  serve     run the internal health listener
  migrate   apply explicit PostgreSQL migrations
  bootstrap create the first administrator and write its token once
  help      print this help text

Flags:
  --config <file>       configuration file; takes precedence over URBINO_CONFIG and ./urbino.yaml
  --output <new-file>   exclusive output file for the one-time administrator token
  --pepper-file <file>  restricted HMAC pepper file; its contents are never logged

Environment:
  URBINO_CONFIG              configuration file used when --config is absent
  URBINO_HEALTH_ADDR         internal health listener address, e.g. 127.0.0.1:9091
  URBINO_PUBLIC_ADDR         public authenticated listener address
  URBINO_ADMIN_ADDR          admin authenticated listener address
  URBINO_AUTH_PEPPER_FILE    restricted HMAC pepper file for runtime authentication
  URBINO_DATABASE_DSN_FILE   restricted file containing the PostgreSQL DSN
Unimplemented later commands fail with a usage error; no command silently succeeds.
`

// Options are the process inputs of Run, including a presence-preserving
// environment snapshot for strict URBINO_ allowlist validation.
type Options struct {
	Args        []string
	Stdout      io.Writer
	Stderr      io.Writer
	LookupEnv   func(string) (string, bool)
	Environment map[string]string
	WorkDir     string
}

func (o *Options) env(name string) string {
	if o.LookupEnv == nil {
		return ""
	}
	v, _ := o.LookupEnv(name)
	return v
}

func (o *Options) environmentValues() map[string]string {
	if o.Environment != nil {
		values := make(map[string]string)
		for name, value := range o.Environment {
			if strings.HasPrefix(name, "URBINO_") {
				values[name] = value
			}
		}
		return values
	}
	values := make(map[string]string)
	if o.LookupEnv == nil {
		return values
	}
	for _, name := range []string{config.EnvConfigPath, config.EnvHealthAddr, config.EnvPublicAddr, config.EnvAdminAddr, config.EnvAuthPepperFile, config.EnvLogLevel, config.EnvEnvironment, config.EnvDatabaseDSNFile} {
		if value, ok := o.LookupEnv(name); ok {
			values[name] = value
		}
	}
	return values
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
	outputPath string
	pepperFile string
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
		case "", "help", "version", "serve", "migrate", "bootstrap":
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
	case "migrate":
		if len(inv.args) > 0 {
			return o.usageError("migrate takes no arguments")
		}
		return o.migrate(ctx, inv.configPath)
	case "bootstrap":
		if len(inv.args) > 0 || inv.outputPath == "" || inv.pepperFile == "" {
			return o.usageError("bootstrap requires --output and --pepper-file and accepts no positional arguments")
		}
		return o.bootstrap(ctx, inv.configPath, inv.outputPath, inv.pepperFile)
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
		case a == "--output":
			if len(rest) == 0 {
				return invocation{}, errors.New("--output requires a file path")
			}
			inv.outputPath, rest = rest[0], rest[1:]
		case strings.HasPrefix(a, "--output="):
			inv.outputPath = strings.TrimPrefix(a, "--output=")
		case a == "--pepper-file":
			if len(rest) == 0 {
				return invocation{}, errors.New("--pepper-file requires a file path")
			}
			inv.pepperFile, rest = rest[0], rest[1:]
		case strings.HasPrefix(a, "--pepper-file="):
			inv.pepperFile = strings.TrimPrefix(a, "--pepper-file=")
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

// migrate loads one configuration and explicitly applies the ordered SQL
// migrations. It is never called by serve or any other startup path.
func (o *Options) migrate(ctx context.Context, configFile string) int {
	path, err := config.ResolvePath(config.PathInput{
		Explicit: configFile,
		Env:      o.env(config.EnvConfigPath),
		Dir:      o.WorkDir,
	})
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}
	cfg, err := config.LoadWithEnvironment(path.File, o.environmentValues())
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}
	dsn, err := config.ReadDatabaseDSN(cfg)
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}
	db, err := postgres.Open(ctx, postgres.Config{DSN: dsn})
	if err != nil {
		o.fail("%s", postgres.SafeErrorMessage(err))
		return ExitError
	}
	defer db.Close()
	migrations, err := migrate.LoadEmbedded()
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}
	if err := (migrate.Runner{Pool: db.Pool()}).Run(ctx, migrations); err != nil {
		o.fail("%v", err)
		return ExitError
	}
	fmt.Fprintf(o.Stdout, "urbino: migrations applied through version %04d\n", migrations[len(migrations)-1].Version)
	return ExitOK
}

func (o *Options) bootstrap(ctx context.Context, configFile, outputPath, pepperFile string) int {
	path, err := config.ResolvePath(config.PathInput{Explicit: configFile, Env: o.env(config.EnvConfigPath), Dir: o.WorkDir})
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}
	cfg, err := config.LoadWithEnvironment(path.File, o.environmentValues())
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}
	dsn, err := config.ReadDatabaseDSN(cfg)
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}
	pepper, err := auth.ReadPepperFile(pepperFile, "admin-v1")
	if err != nil {
		o.fail("administrator bootstrap secret configuration is invalid")
		return ExitError
	}
	db, err := postgres.Open(ctx, postgres.Config{DSN: dsn})
	if err != nil {
		o.fail("%s", postgres.SafeErrorMessage(err))
		return ExitError
	}
	defer db.Close()
	if err := postgres.BootstrapAdmin(ctx, db, auth.Issuer{Current: pepper}, outputPath, time.Now().UTC()); err != nil {
		o.fail("%v", err)
		return ExitError
	}
	fmt.Fprintf(o.Stdout, "urbino: administrator bootstrap token written to %s\n", outputPath)
	return ExitOK
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

	environment := o.environmentValues()
	cfg, err := config.LoadWithEnvironment(path.File, environment)
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}
	var db *postgres.DB
	if cfg.Database.DSNFile != "" {
		dsn, err := config.ReadDatabaseDSN(cfg)
		if err != nil {
			o.fail("%v", err)
			return ExitError
		}
		db, err = postgres.Open(ctx, postgres.Config{DSN: dsn})
		if err != nil {
			o.fail("%s", postgres.SafeErrorMessage(err))
			return ExitError
		}
		defer db.Close()
		migrations, err := migrate.LoadEmbedded()
		if err != nil {
			o.fail("%v", err)
			return ExitError
		}
		if err := migrate.CheckCompatibility(ctx, db.Pool(), migrations); err != nil {
			o.fail("%v", err)
			return ExitError
		}
	}

	healthAddr := config.ResolveHealthAddr(cfg, o.env(config.EnvHealthAddr))
	health, err := httpapi.NewHealthServer(healthAddr)
	if err != nil {
		o.fail("%v", err)
		return ExitError
	}
	if err := health.Listen(); err != nil {
		o.fail("%v", err)
		return ExitError
	}
	defer func() { _ = health.Close() }()

	publicAddr := config.ResolvePublicAddr(cfg, o.env(config.EnvPublicAddr))
	adminAddr := config.ResolveAdminAddr(cfg, o.env(config.EnvAdminAddr))
	pepperFile := config.ResolveAuthPepperFile(cfg, o.env(config.EnvAuthPepperFile))
	var authenticated *httpapi.AuthenticatedServers
	if publicAddr != "" || adminAddr != "" || pepperFile != "" {
		if db == nil || publicAddr == "" || adminAddr == "" || pepperFile == "" {
			o.fail("public/admin listeners require database, public_addr, admin_addr and auth_pepper_file")
			return ExitError
		}
		pepper, err := auth.ReadPepperFile(pepperFile, "admin-v1")
		if err != nil {
			o.fail("runtime authentication secret configuration is invalid")
			return ExitError
		}
		authenticator, err := postgres.NewAuthenticator(db, pepper, nil)
		if err != nil {
			o.fail("runtime authentication configuration is invalid")
			return ExitError
		}
		authenticated, err = httpapi.NewAuthenticatedServers(publicAddr, adminAddr, authenticator.AuthenticatePublic, authenticator.AuthenticateAdmin)
		if err != nil {
			o.fail("%v", err)
			return ExitError
		}
		if err := authenticated.Listen(); err != nil {
			o.fail("%v", err)
			return ExitError
		}
		defer func() { _ = authenticated.Close() }()
	}

	fmt.Fprintf(o.Stderr, "urbino: config %s (source %s)\n", path.File, path.Source)
	fmt.Fprintf(o.Stderr, "urbino: internal health listener on %s, GET %s only\n", health.Addr(), httpapi.HealthPath)
	if authenticated != nil {
		fmt.Fprintf(o.Stderr, "urbino: public listener on %s, GET %s\n", authenticated.PublicAddr(), httpapi.PublicIdentityPath)
		fmt.Fprintf(o.Stderr, "urbino: admin listener on %s, GET %s\n", authenticated.AdminAddr(), httpapi.AdminIdentityPath)
	}

	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	servers := 1
	errCh := make(chan error, 2)
	go func() { errCh <- health.Serve(serveCtx) }()
	if authenticated != nil {
		servers++
		go func() { errCh <- authenticated.Serve(serveCtx) }()
	}
	var serveErr error
	for i := 0; i < servers; i++ {
		err := <-errCh
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && serveErr == nil {
			serveErr = err
		}
		if err != nil {
			cancel()
		}
	}
	if serveErr != nil {
		o.fail("%v", serveErr)
		return ExitError
	}
	if ctx.Err() != nil {
		return ExitOK
	}
	fmt.Fprintln(o.Stderr, "urbino: listeners stopped")
	return ExitOK
}
