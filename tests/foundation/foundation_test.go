// Stage 00 foundation behaviour tests.
//
// These tests are independent of the implementation unit tests: they build the
// real cmd/urbino binary and observe it as a black box (exit codes, streams,
// listening sockets) and exercise the health listener over real TCP. No
// production code is modified and no real upstream service is contacted.
package foundation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"example.com/urbino/internal/httpapi"
)

// urbinoBin is the path of the binary built by TestMain.
var urbinoBin string

func TestMain(m *testing.M) {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "foundation tests: %v\n", err)
		os.Exit(1)
	}
	binDir, err := os.MkdirTemp("", "urbino-foundation-bin-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "foundation tests: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(binDir)

	// The binary must be buildable before any behaviour can be tested; a
	// failed build aborts the whole run with the compiler output.
	urbinoBin = filepath.Join(binDir, "urbino")
	build := exec.Command("go", "build", "-o", urbinoBin, "./cmd/urbino")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "foundation tests: go build ./cmd/urbino failed: %v\n%s", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// findRepoRoot walks up from the test directory to the module root.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found above the test directory")
		}
		dir = parent
	}
}

// cleanEnv inherits the environment minus any URBINO_* variables so tests
// control the configuration inputs deterministically.
func cleanEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "URBINO_") {
			env = append(env, kv)
		}
	}
	return env
}

// runUrbino runs one urbino command and returns its real exit code and streams.
func runUrbino(t *testing.T, dir string, env []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(urbinoBin, args...)
	cmd.Dir = dir
	cmd.Env = append(cleanEnv(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
	default:
		t.Fatalf("run urbino %v: %v", args, err)
	}
	code := 0
	if exitErr != nil {
		code = exitErr.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

// serveProc is one running `urbino serve` subprocess.
type serveProc struct {
	cmd     *exec.Cmd
	stdout  *bytes.Buffer
	stderr  *bytes.Buffer
	stopped bool
}

// startServe starts `urbino serve` in dir with the given env and extra args and
// registers cleanup that terminates the process.
func startServe(t *testing.T, dir string, env []string, args ...string) *serveProc {
	t.Helper()
	cmd := exec.Command(urbinoBin, append([]string{"serve"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(cleanEnv(), env...)
	sp := &serveProc{cmd: cmd, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	cmd.Stdout, cmd.Stderr = sp.stdout, sp.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start urbino serve: %v", err)
	}
	t.Cleanup(sp.stop)
	return sp
}

// stop terminates the process gracefully and waits for it, so captured streams
// are safe to read afterwards.
func (sp *serveProc) stop() {
	if sp.stopped {
		return
	}
	sp.stopped = true
	_ = sp.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_ = sp.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = sp.cmd.Process.Kill()
		<-done
	}
}

func (sp *serveProc) diagnostics() string {
	return strings.TrimSpace(sp.stderr.String())
}

// writeConfig writes a minimal urbino.yaml selecting healthAddr.
func writeConfig(t *testing.T, path, healthAddr string) {
	t.Helper()
	content := fmt.Sprintf("health_addr: %q\n", healthAddr)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// distinctPorts returns n distinct reserved loopback ports.
func distinctPorts(t *testing.T, n int) []int {
	t.Helper()
	ports := make([]int, 0, n)
	seen := make(map[int]bool, n)
	for len(ports) < n {
		p := freePort(t)
		if !seen[p] {
			seen[p] = true
			ports = append(ports, p)
		}
	}
	return ports
}

func addr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// portOpen reports whether something currently accepts TCP connections on addr.
func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func waitPortOpen(t *testing.T, address string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if portOpen(address) {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

// expectPortClosed fails if anything starts listening on address within window.
func expectPortClosed(t *testing.T, address string, window time.Duration) {
	t.Helper()
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		if portOpen(address) {
			t.Fatalf("unexpected listener on %s", address)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestVersionCommandPrintsReadableVersion(t *testing.T) {
	code, stdout, stderr := runUrbino(t, t.TempDir(), nil, "version")
	if code != 0 {
		t.Fatalf("urbino version exit = %d, want 0; stderr: %q", code, stderr)
	}
	fields := strings.Fields(stdout)
	if len(fields) != 3 || fields[0] != "urbino" || fields[1] != "version" || fields[2] == "" {
		t.Fatalf("urbino version output not readable: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("urbino version wrote diagnostics to stderr: %q", stderr)
	}
}

func TestUnknownCommandExitsNonZero(t *testing.T) {
	const bogus = "definitely-not-a-command"
	code, _, stderr := runUrbino(t, t.TempDir(), nil, bogus)
	if code == 0 {
		t.Fatalf("unknown command %q exited 0, want non-zero", bogus)
	}
	if !strings.Contains(stderr, bogus) {
		t.Fatalf("stderr does not name the unknown command: %q", stderr)
	}
}
func TestUnknownCommandWithHelpExitsNonZero(t *testing.T) {
	for _, command := range []string{"definitely-not-a-command", "worker", "admin"} {
		code, _, stderr := runUrbino(t, t.TempDir(), nil, command, "--help")
		if code == 0 {
			t.Fatalf("%s --help exited 0, want non-zero", command)
		}
		if !strings.Contains(stderr, command) {
			t.Fatalf("stderr does not name %s: %q", command, stderr)
		}
	}
}

func TestServeWithMissingExplicitConfigFails(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-such-urbino.yaml")
	code, _, stderr := runUrbino(t, dir, nil, "serve", "--config", missing)
	if code == 0 {
		t.Fatal("serve with missing --config file exited 0, want non-zero")
	}
	if !strings.Contains(stderr, "no-such-urbino.yaml") {
		t.Fatalf("error output does not name the missing file: %q", stderr)
	}
}

// A URBINO_CONFIG file that does not exist must fail even though a valid
// default ./urbino.yaml is present: sources are never merged or fallen back.
func TestServeWithMissingEnvConfigDoesNotFallBack(t *testing.T) {
	dir := t.TempDir()
	defaultPort := freePort(t)
	writeConfig(t, filepath.Join(dir, "urbino.yaml"), addr(defaultPort))
	missing := filepath.Join(dir, "env-selected-missing.yaml")

	code, _, stderr := runUrbino(t, dir, []string{"URBINO_CONFIG=" + missing}, "serve")
	if code == 0 {
		t.Fatal("serve with missing URBINO_CONFIG file exited 0, want non-zero")
	}
	if !strings.Contains(stderr, "env-selected-missing.yaml") {
		t.Fatalf("error output does not name the URBINO_CONFIG file: %q", stderr)
	}
	if portOpen(addr(defaultPort)) {
		t.Fatalf("serve bound %s despite failing on the missing URBINO_CONFIG file", addr(defaultPort))
	}
}

// A mistyped configuration must fail loudly instead of being ignored.
func TestServeWithInvalidConfigFails(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"wrong value type", "health_addr: [9000]\n"},
		{"unknown field", "not_a_setting: true\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "urbino.yaml"), []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			code, _, stderr := runUrbino(t, dir, nil, "serve")
			if code == 0 {
				t.Fatalf("serve accepted invalid configuration %q (exit 0)", tc.yaml)
			}
			if stderr == "" {
				t.Fatal("serve rejected invalid configuration without a diagnostic")
			}
		})
	}
}

// --config selects the configuration file even when a default ./urbino.yaml
// exists in the working directory; both are never merged.
func TestConfigFlagOverridesDefaultFile(t *testing.T) {
	dir := t.TempDir()
	ports := distinctPorts(t, 2)
	flagPort, defaultPort := ports[0], ports[1]

	writeConfig(t, filepath.Join(dir, "urbino.yaml"), addr(defaultPort))
	flagFile := filepath.Join(dir, "explicit.yaml")
	writeConfig(t, flagFile, addr(flagPort))

	sp := startServe(t, dir, nil, "--config", flagFile)
	if !waitPortOpen(t, addr(flagPort), 5*time.Second) {
		sp.stop()
		t.Fatalf("serve did not listen on the --config address %s; stderr:\n%s", addr(flagPort), sp.diagnostics())
	}
	expectPortClosed(t, addr(defaultPort), 500*time.Millisecond)
}

// URBINO_HEALTH_ADDR wins over the health_addr of the selected configuration
// file.
func TestHealthAddrEnvOverridesConfigFile(t *testing.T) {
	dir := t.TempDir()
	ports := distinctPorts(t, 2)
	filePort, envPort := ports[0], ports[1]

	writeConfig(t, filepath.Join(dir, "urbino.yaml"), addr(filePort))
	env := []string{"URBINO_HEALTH_ADDR=" + addr(envPort)}

	sp := startServe(t, dir, env)
	if !waitPortOpen(t, addr(envPort), 5*time.Second) {
		sp.stop()
		t.Fatalf("serve did not listen on the URBINO_HEALTH_ADDR address %s; stderr:\n%s", addr(envPort), sp.diagnostics())
	}
	expectPortClosed(t, addr(filePort), 500*time.Millisecond)
}

// The internal health listener serves exactly GET /healthz: no data plane and
// no admin plane route exists, and non-GET methods are rejected.
func TestInternalHealthListenerSurface(t *testing.T) {
	srv, err := httpapi.NewHealthServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("create health server: %v", err)
	}
	if err := srv.Listen(); err != nil {
		t.Fatalf("bind health listener: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ctx)
	}()
	baseURL := "http://" + srv.Addr().String()
	if !waitPortOpen(t, srv.Addr().String(), 5*time.Second) {
		t.Fatal("health listener never accepted connections")
	}

	t.Run("get healthz returns 200 json ok", func(t *testing.T) {
		resp, err := http.Get(baseURL + httpapi.HealthPath)
		if err != nil {
			t.Fatalf("GET /healthz: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /healthz status = %d, want 200", resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Fatalf("GET /healthz Content-Type = %q, want application/json", got)
		}
		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("GET /healthz body is not JSON: %v", err)
		}
		if body.Status != "ok" {
			t.Fatalf("GET /healthz status field = %q, want %q", body.Status, "ok")
		}
	})

	t.Run("data and admin planes do not exist", func(t *testing.T) {
		for _, path := range []string{"/v1/models", "/admin/tenants"} {
			resp, err := http.Get(baseURL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			status := resp.StatusCode
			resp.Body.Close()
			if status != http.StatusNotFound {
				t.Fatalf("GET %s status = %d, want 404", path, status)
			}
		}
	})

	t.Run("non-get healthz is rejected", func(t *testing.T) {
		for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodHead} {
			req, err := http.NewRequest(method, baseURL+httpapi.HealthPath, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s /healthz: %v", method, err)
			}
			status := resp.StatusCode
			resp.Body.Close()
			if status == http.StatusOK {
				t.Fatalf("%s /healthz status = 200, want rejection", method)
			}
			if status != http.StatusMethodNotAllowed {
				t.Fatalf("%s /healthz status = %d, want 405", method, status)
			}
		}
	})

	cancel()
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve after context cancellation = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}
}
