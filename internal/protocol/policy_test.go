package protocol

import (
	"errors"
	"testing"
)

func TestCapabilityAuthAndDisabledAreDistinct(t *testing.T) {
	c := CapabilityDescriptor{Name: "POST /v1/chat/completions", Version: "v1", Enabled: false, RequiresAuthorization: true}
	if !errors.Is(CheckCapability(c, false), ErrUnauthorized) {
		t.Fatal("unauthenticated request must be unauthorized")
	}
	if !errors.Is(CheckCapability(c, true), ErrUnsupportedCapability) {
		t.Fatal("authenticated disabled capability must be unsupported")
	}
	c.Enabled = true
	if err := CheckCapability(c, true); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownEndpointIsNotAdvertised(t *testing.T) {
	if _, ok := CapabilityForEndpoint("POST", "/v1/not-declared"); ok {
		t.Fatal("undeclared endpoint advertised")
	}
}

func TestFieldPolicyUnknownAndBoundedSize(t *testing.T) {
	p, _ := FindEndpointPolicy("POST", "/v1/chat/completions")
	if _, err := ValidateObjectFields([]byte(`{"model":"m","made_up":1}`), p); !errors.Is(err, ErrUnknownField) {
		t.Fatalf("unknown accepted: %v", err)
	}
	tooLarge := make([]byte, 16<<10)
	for i := range tooLarge {
		tooLarge[i] = 'x'
	}
	body := append([]byte(`{"model":"m","metadata":"`), tooLarge...)
	body = append(body, []byte(`"}`)...)
	if _, err := ValidateObjectFields(body, p); !errors.Is(err, ErrFieldTooLarge) {
		t.Fatalf("oversized bounded field accepted: %v", err)
	}
}
