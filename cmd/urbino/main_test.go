package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/starhui-dev/urbino/internal/securitylog"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"version"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Urbino dev") {
		t.Fatalf("version output = %q", out.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"wat"}, &out, &errOut); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(errOut.String(), "Urbino") {
		t.Fatalf("usage output = %q", errOut.String())
	}
}

func TestHealthRouter(t *testing.T) {
	request := httptest.NewRequest("GET", "/healthz", nil)
	recorder := httptest.NewRecorder()
	healthRouter().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"ok"`) {
		t.Fatalf("health response = %d %s", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest("GET", "/v1/models", nil)
	recorder = httptest.NewRecorder()
	healthRouter().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("model endpoint status = %d, want 404", recorder.Code)
	}
}

func TestServeRejectsUnavailableAddress(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := serve([]string{"--health-addr", "not-a-valid-address"}, &out, &errOut); err == nil {
		t.Fatal("serve() unexpectedly succeeded with an invalid listener address")
	}
	if out.Len() != 0 {
		t.Fatalf("serve() announced startup before binding: %q", out.String())
	}
}

func TestServeHealthStopsOnCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := securitylog.New(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	result := make(chan error, 1)
	go func() { result <- serveHealth(ctx, listener, io.Discard, logger) }()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", response.StatusCode)
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serveHealth() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serveHealth() did not stop after cancellation")
	}
	if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("listener remained open after shutdown: %v", err)
	}
}

func TestServeHealthReturnsListenerFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	logger := securitylog.New(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err := serveHealth(context.Background(), listener, io.Discard, logger); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("serveHealth() error = %v, want closed listener", err)
	}
}
