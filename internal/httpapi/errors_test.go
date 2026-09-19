package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/urbino/internal/domain"
)

func TestWriteUnsupportedReturnsStableNonSuccess(t *testing.T) {
	recorder := httptest.NewRecorder()
	requestID, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	WriteUnsupported(recorder, requestID, "responses")
	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotImplemented)
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", recorder.Header().Get("Content-Type"))
	}
	var response publicError
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != domain.CodeUnsupportedCapability || response.RequestID != requestID.String() || response.Retryable {
		t.Fatalf("unexpected unsupported response: %+v", response)
	}
	if response.Details == nil {
		t.Fatal("details must be a safe object")
	}
}
