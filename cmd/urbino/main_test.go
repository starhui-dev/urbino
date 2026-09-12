package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
