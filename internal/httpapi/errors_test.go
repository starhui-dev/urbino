package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/urbino/internal/domain"
)

func TestWriteUnsupportedReturnsStableNonSuccess(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteUnsupported(recorder, domain.UUID{}, "responses")
	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotImplemented)
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Body.String() == "" {
		t.Fatal("empty error response")
	}
}
