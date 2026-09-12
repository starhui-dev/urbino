package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	urbinoConfig "github.com/starhui-dev/urbino/internal/config"
	"github.com/starhui-dev/urbino/internal/securitylog"
	"github.com/starhui-dev/urbino/internal/storage/postgres"
)

var (
	version = "dev"
	commit  = "unknown"
)

const serviceName = "Urbino"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return fmt.Errorf("missing command")
	}
	switch args[0] {
	case "version":
		_, err := fmt.Fprintf(stdout, "%s %s (%s)\n", serviceName, version, commit)
		return err
	case "serve":
		return serve(args[1:], stdout, stderr)
	case "config":
		return configCommand(args[1:], stdout, stderr)
	case "migrate":
		return migrateCommand(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func serve(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "configuration file")
	healthAddr := flags.String("health-addr", envOrDefault("URBINO_HEALTH_ADDR", ""), "internal health listener address")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("serve: unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	cfg, err := loadRuntimeConfig(*configPath)
	if err != nil {
		return err
	}
	dsn, err := databaseURL(cfg.Database)
	if err != nil {
		return err
	}
	if dsn != "" {
		checkCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		pool, openErr := postgres.Open(checkCtx, dsn)
		cancel()
		if openErr != nil {
			return openErr
		}
		defer pool.Close()
	}
	if *healthAddr == "" {
		*healthAddr = cfg.Server.Internal.Listen
	}
	if *healthAddr == "" {
		*healthAddr = "127.0.0.1:9091"
	}

	logger := securitylog.New(slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	listener, err := net.Listen("tcp", *healthAddr)
	if err != nil {
		return fmt.Errorf("health listener: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveHealth(ctx, listener, stdout, logger)
}

func configCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "validate" {
		return fmt.Errorf("config: expected validate")
	}
	flags := flag.NewFlagSet("config validate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("config", "", "configuration file")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("config validate: unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	resolved, explicit, err := urbinoConfig.ResolvePath(*path, os.Environ(), currentDir())
	if err != nil {
		return err
	}
	if _, err := os.Stat(resolved); err != nil {
		if explicit {
			return fmt.Errorf("config %q: %w", resolved, err)
		}
		return fmt.Errorf("config %q is missing", resolved)
	}
	if _, err := urbinoConfig.LoadPath(resolved, explicit); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "configuration valid: %s\n", resolved)
	return err
}

func loadRuntimeConfig(flagPath string) (urbinoConfig.Config, error) {
	path, explicit, err := urbinoConfig.ResolvePath(flagPath, os.Environ(), currentDir())
	if err != nil {
		return urbinoConfig.Config{}, err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		if explicit {
			return urbinoConfig.Config{}, fmt.Errorf("config %q: %w", path, statErr)
		}
		return urbinoConfig.Default(), nil
	}
	return urbinoConfig.LoadPath(path, explicit)
}

func currentDir() string {
	d, err := os.Getwd()
	if err != nil {
		return "."
	}
	return d
}

func serveHealth(ctx context.Context, listener net.Listener, stdout io.Writer, logger securitylog.Logger) error {
	server := &http.Server{Handler: healthRouter(), ReadHeaderTimeout: 5 * time.Second}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()

	logger.Event("serve_started", listener.Addr().String())
	_, _ = fmt.Fprintf(stdout, "%s serving internal health on %s\n", serviceName, listener.Addr())
	select {
	case err := <-serveErrors:
		if err == http.ErrServerClosed {
			return nil
		}
		logger.Error("serve_failed", err)
		return fmt.Errorf("health server: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func healthRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": serviceName})
	})
	return r
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Urbino - backend AI gateway")
	fmt.Fprintln(w, "用法: urbino <version|serve|migrate|config validate|help>")
}
