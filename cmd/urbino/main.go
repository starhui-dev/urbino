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
	"github.com/starhui-dev/urbino/internal/securitylog"
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
	healthAddr := flags.String("health-addr", envOrDefault("URBINO_HEALTH_ADDR", "127.0.0.1:9091"), "internal health listener address")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("serve: unexpected arguments: %s", strings.Join(flags.Args(), " "))
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
	fmt.Fprintln(w, "用法: urbino <version|serve|help>")
}
