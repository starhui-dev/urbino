package cli

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/urbino/internal/config"
	"example.com/urbino/internal/version"
)

// syncBuffer is a goroutine-safe writer for capturing command output while a
// command is still running.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// envMap turns a map into the Options.LookupEnv signature.
func envMap(vars map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := vars[name]
		return v, ok
	}
}

func runCLI(t *testing.T, ctx context.Context, workDir string, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr syncBuffer
	code := Run(ctx, Options{
		Args:      args,
		Stdout:    &stdout,
		Stderr:    &stderr,
		LookupEnv: envMap(env),
		WorkDir:   workDir,
	})
	return code, stdout.String(), stderr.String()
}

func writeConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestVersionCommand(t *testing.T) {
	code, stdout, stderr := runCLI(t, context.Background(), t.TempDir(), nil, "version")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	want := "urbino version " + version.String()
	if !strings.Contains(stdout, want) {
		t.Fatalf("stdout = %q, want it to contain %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestVersionRejectsArguments(t *testing.T) {
	code, _, stderr := runCLI(t, context.Background(), t.TempDir(), nil, "version", "extra")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "version takes no arguments") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestUnknownCommandFails(t *testing.T) {
	for _, command := range []string{"bogus", "versionn", "worker", "migrate", "bootstrap", "admin", "doctor"} {
		code, stdout, stderr := runCLI(t, context.Background(), t.TempDir(), nil, command)
		if code != ExitUsage {
			t.Fatalf("%s: exit code = %d, want %d", command, code, ExitUsage)
		}
		if !strings.Contains(stderr, "unknown command") {
			t.Fatalf("%s: stderr = %q, want an unknown command error", command, stderr)
		}
		if stdout != "" {
			t.Fatalf("%s: stdout = %q, want empty", command, stdout)
		}
	}
}
func TestUnknownCommandWithHelpFails(t *testing.T) {
	for _, command := range []string{"bogus", "worker", "admin"} {
		code, stdout, stderr := runCLI(t, context.Background(), t.TempDir(), nil, command, "--help")
		if code != ExitUsage {
			t.Fatalf("%s --help: exit code = %d, want %d", command, code, ExitUsage)
		}
		if !strings.Contains(stderr, "unknown command") {
			t.Fatalf("%s --help: stderr = %q, want unknown command", command, stderr)
		}
		if stdout != "" {
			t.Fatalf("%s --help: stdout = %q, want empty", command, stdout)
		}
	}
}

func TestNoCommandPrintsUsageOnStderr(t *testing.T) {
	code, stdout, stderr := runCLI(t, context.Background(), t.TempDir(), nil)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Fatalf("stderr = %q, want usage text", stderr)
	}
}

func TestHelpFlagSucceeds(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}} {
		code, stdout, stderr := runCLI(t, context.Background(), t.TempDir(), nil, args...)
		if code != ExitOK {
			t.Fatalf("%v: exit code = %d, want %d (stderr: %s)", args, code, ExitOK, stderr)
		}
		if !strings.Contains(stdout, "Usage:") {
			t.Fatalf("%v: stdout = %q, want usage text", args, stdout)
		}
	}
}

func TestUnknownFlagFails(t *testing.T) {
	code, _, stderr := runCLI(t, context.Background(), t.TempDir(), nil, "--nope", "version")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestServeWithoutConfigFails(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLI(t, context.Background(), dir, nil, "serve")
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, config.DefaultFileName) || !strings.Contains(stderr, "does not exist") {
		t.Fatalf("stderr = %q, want a missing configuration error naming %s", stderr, config.DefaultFileName)
	}
}

func TestServeMissingExplicitConfigDoesNotFallBack(t *testing.T) {
	dir := t.TempDir()
	envFile := writeConfig(t, dir, "env.yaml", "health_addr: 127.0.0.1:0\n")
	env := map[string]string{config.EnvConfigPath: envFile}

	code, _, stderr := runCLI(t, context.Background(), dir, env, "serve", "--config", filepath.Join(dir, "missing.yaml"))
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "missing.yaml") {
		t.Fatalf("stderr = %q, want the explicit path to be reported", stderr)
	}
	if strings.Contains(stderr, "env.yaml") {
		t.Fatalf("stderr = %q, want no fall back to URBINO_CONFIG", stderr)
	}
}

func TestServeConfigSelectionAndInvalidAddress(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, config.DefaultFileName, "health_addr: 127.0.0.1:0\n")
	envFile := writeConfig(t, dir, "env.yaml", "health_addr: not-an-address\n")
	env := map[string]string{config.EnvConfigPath: envFile}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// URBINO_CONFIG wins over ./urbino.yaml, and its invalid address is reported
	// instead of being ignored: the error can only come from env.yaml.
	code, _, stderr := runCLI(t, ctx, dir, env, "serve")
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, ExitError, stderr)
	}
	if !strings.Contains(stderr, `invalid health listener address "not-an-address"`) {
		t.Fatalf("stderr = %q, want the URBINO_CONFIG address to be selected", stderr)
	}
}

func TestServeRejectsUnknownConfigField(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, config.DefaultFileName, "healt_addr: 127.0.0.1:0\n")

	code, _, stderr := runCLI(t, context.Background(), dir, nil, "serve")
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "decode") {
		t.Fatalf("stderr = %q, want a configuration decode error", stderr)
	}
}

func TestServeReportsBindFailure(t *testing.T) {
	dir := t.TempDir()
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer occupied.Close()
	writeConfig(t, dir, config.DefaultFileName, "health_addr: "+occupied.Addr().String()+"\n")

	code, _, stderr := runCLI(t, context.Background(), dir, nil, "serve")
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, ExitError, stderr)
	}
	if !strings.Contains(stderr, "listen on") {
		t.Fatalf("stderr = %q, want a bind failure", stderr)
	}
}

// TestServeHealthListenerEndToEnd runs the real command with the real listener
// address, checks that only GET /healthz is answered and stops the command the
// way a signal would, by cancelling the context.
func TestServeHealthListenerEndToEnd(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, config.DefaultFileName, "health_addr: 127.0.0.1:0\n")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- Run(ctx, Options{
			Args:      []string{"serve"},
			Stdout:    &stdout,
			Stderr:    &stderr,
			LookupEnv: envMap(nil),
			WorkDir:   dir,
		})
	}()

	// The command reports the bound address, which is the only way a caller can
	// discover the ephemeral port.
	addrRe := regexp.MustCompile(`internal health listener on ([0-9.:\[\]]+)`)
	var addr string
	for range 200 {
		if m := addrRe.FindStringSubmatch(stderr.String()); m != nil {
			addr = m[1]
			break
		}
		select {
		case code := <-done:
			t.Fatalf("serve exited early with code %d (stderr: %s)", code, stderr.String())
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == "" {
		t.Fatalf("serve never reported its listener address (stderr: %s)", stderr.String())
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("GET /healthz = %d %q, want 200 with status ok", resp.StatusCode, body)
	}

	resp, err = client.Get("http://" + addr + "/v1/models")
	if err != nil {
		t.Fatalf("GET /v1/models: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /v1/models = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	cancel()
	select {
	case code := <-done:
		if code != ExitOK {
			t.Fatalf("serve exit code after cancellation = %d, want %d (stderr: %s)", code, ExitOK, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after context cancellation")
	}
}

func TestServeHealthAddrEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, config.DefaultFileName, "health_addr: not-an-address\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	env := map[string]string{config.EnvHealthAddr: "also-not-an-address"}
	code, _, stderr := runCLI(t, ctx, dir, env, "serve")
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "also-not-an-address") {
		t.Fatalf("stderr = %q, want the environment address to win over the configuration file", stderr)
	}
}
