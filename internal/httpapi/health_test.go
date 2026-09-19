package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthHandlerServesOnlyGetHealthz(t *testing.T) {
	handler := NewHealthHandler()

	t.Run("GET /healthz", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, HealthPath, nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("Content-Type = %q, want application/json", ct)
		}
		var body struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
		}
		if body.Status != "ok" {
			t.Fatalf("status field = %q, want ok", body.Status)
		}
	})

	t.Run("other methods on /healthz are rejected", func(t *testing.T) {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead} {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(method, HealthPath, strings.NewReader("{}")))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s status = %d, want %d", method, HealthPath, rec.Code, http.StatusMethodNotAllowed)
			}
		}
	})

	t.Run("no public, admin or model route exists", func(t *testing.T) {
		paths := []string{
			"/",
			"/healthz/extra",
			"/metrics",
			"/v1/models",
			"/v1/chat/completions",
			"/api/admin/tenants",
			"/admin/health",
		}
		for _, path := range paths {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("GET %s status = %d, want %d", path, rec.Code, http.StatusNotFound)
			}
		}
	})
}

func TestNewHealthServerValidatesAddress(t *testing.T) {
	for _, addr := range []string{"", "  ", "127.0.0.1", "not-an-address"} {
		if _, err := NewHealthServer(addr); err == nil {
			t.Fatalf("NewHealthServer(%q) accepted an invalid address", addr)
		}
	}
}

func TestHealthServerServesAndStopsWithContext(t *testing.T) {
	srv, err := NewHealthServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewHealthServer: %v", err)
	}
	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if srv.Addr() == nil {
		t.Fatal("Addr() is nil after Listen")
	}
	if err := srv.Listen(); err == nil {
		t.Fatal("second Listen was accepted")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()

	url := "http://" + srv.Addr().String() + HealthPath
	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error
	served := false
	for range 100 {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, http.StatusOK)
			}
			served = true
			break
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	if !served {
		t.Fatalf("GET %s never succeeded: %v", url, lastErr)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v, want nil after cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}

	if _, err := net.DialTimeout("tcp", srv.Addr().String(), 200*time.Millisecond); err == nil {
		t.Fatal("listener still accepts connections after shutdown")
	}
}

func TestServeBeforeCancelledContext(t *testing.T) {
	srv, err := NewHealthServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewHealthServer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = srv.Serve(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve = %v, want context.Canceled", err)
	}
	if srv.Addr() != nil {
		t.Fatal("Serve bound the listener for an already cancelled context")
	}
}

func TestServeClosesPreboundListenerWhenContextAlreadyCancelled(t *testing.T) {
	srv, err := NewHealthServer("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	addr := srv.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.Serve(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve = %v, want context.Canceled", err)
	}
	if _, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
		t.Fatal("prebound listener remained open after cancelled Serve")
	}
}

func TestHealthServerReportsBindFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer occupied.Close()

	srv, err := NewHealthServer(occupied.Addr().String())
	if err != nil {
		t.Fatalf("NewHealthServer: %v", err)
	}
	if err := srv.Listen(); err == nil {
		t.Fatal("Listen succeeded on an occupied address")
	}
}
