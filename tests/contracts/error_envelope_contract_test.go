package contracts_test

// P01-T06 (checklists/test-matrix.csv: 能力契约): unsupported,
// unauthorized and unknown are the stable public error classes, and the
// public envelope carries exactly the contractual
// code/message/request_id/retryable/details fields.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"example.com/urbino/internal/domain"
)

func TestP01T06ErrorClassesAreStable(t *testing.T) {
	classes := []struct {
		class domain.ErrorClass
		want  string
	}{
		{domain.ClassUnsupported, "unsupported"},
		{domain.ClassUnauthorized, "unauthorized"},
		{domain.ClassUnknown, "unknown"},
	}
	seen := map[domain.ErrorClass]bool{}
	for _, c := range classes {
		if string(c.class) != c.want {
			t.Errorf("error class = %q, want stable %q", string(c.class), c.want)
		}
		if seen[c.class] {
			t.Errorf("duplicate error class %q", string(c.class))
		}
		seen[c.class] = true
	}
	if len(seen) != 3 {
		t.Fatalf("three distinct public error classes are required, got %d", len(seen))
	}
}

func TestP01T06ClassifiedConstructors(t *testing.T) {
	cases := []struct {
		name  string
		err   *domain.Error
		class domain.ErrorClass
		code  domain.Code
	}{
		{"unsupported", domain.Unsupported("capability not enabled"), domain.ClassUnsupported, domain.CodeUnsupportedCapability},
		{"unauthorized", domain.Unauthorized("missing admin credential"), domain.ClassUnauthorized, domain.CodeUnauthorized},
		{"unknown", domain.Unknown("uncertain upstream outcome"), domain.ClassUnknown, domain.CodeUnknown},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Class != tt.class {
				t.Errorf("class = %q, want %q", tt.err.Class, tt.class)
			}
			if tt.err.Code != tt.code {
				t.Errorf("code = %q, want %q", tt.err.Code, tt.code)
			}
			if tt.err.Retryable {
				t.Errorf("classified errors must default to non-retryable")
			}
			var target *domain.Error
			if !errors.As(fmt.Errorf("dispatch: %w", tt.err), &target) {
				t.Fatalf("classified error must stay recoverable through errors.As")
			}
			if target.Code != tt.code {
				t.Errorf("recovered code = %q, want %q", target.Code, tt.code)
			}
		})
	}
	// Stable public codes the OpenAPI contract enumerates.
	if string(domain.CodeUnsupportedCapability) != "unsupported_capability" {
		t.Errorf("unsupported code = %q, want unsupported_capability", domain.CodeUnsupportedCapability)
	}
	if string(domain.CodeUnauthorized) != "unauthorized" {
		t.Errorf("unauthorized code = %q, want unauthorized", domain.CodeUnauthorized)
	}
	if string(domain.CodeUnknown) != "unknown" {
		t.Errorf("unknown code = %q, want unknown", domain.CodeUnknown)
	}
}

func TestP01T06PublicErrorEnvelopeJSON(t *testing.T) {
	id, err := domain.ParseUUID("00000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatalf("parse fixed uuid: %v", err)
	}
	env := domain.PublicError{
		Code:      domain.CodeUnsupportedCapability,
		Message:   "capability not enabled",
		RequestID: id,
		Retryable: false,
		Details:   map[string]string{"capability": "responses"},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("envelope must marshal to a JSON object: %v", err)
	}

	gotKeys := make([]string, 0, len(out))
	for k := range out {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	wantKeys := "code,details,message,request_id,retryable"
	if strings.Join(gotKeys, ",") != wantKeys {
		t.Fatalf("public envelope keys = [%s], want exactly [%s] (raw: %s)",
			strings.Join(gotKeys, ","), wantKeys, raw)
	}
	if out["code"] != "unsupported_capability" {
		t.Errorf("code = %#v, want unsupported_capability", out["code"])
	}
	if out["retryable"] != false {
		t.Errorf("retryable = %#v, want a JSON boolean false", out["retryable"])
	}
	if out["request_id"] != "00000000-0000-4000-8000-000000000001" {
		t.Errorf("request_id = %#v, want the canonical UUID string (raw: %s)", out["request_id"], raw)
	}
	details, ok := out["details"].(map[string]any)
	if !ok {
		t.Fatalf("details = %#v, want a JSON object of safe string values (raw: %s)", out["details"], raw)
	}
	if details["capability"] != "responses" {
		t.Errorf("details[capability] = %#v, want responses", details["capability"])
	}
}
