package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
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
	server := &http.Server{Addr: *healthAddr, Handler: healthRouter(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Event("serve_started", *healthAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("serve_failed", err)
		}
	}()

	_, _ = fmt.Fprintf(stdout, "%s serving internal health on %s\n", serviceName, *healthAddr)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
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
