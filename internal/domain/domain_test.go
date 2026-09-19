package domain

import (
	"math"
	"testing"
)

func TestParseMoneyRejectsNegativePrecisionAndOverflow(t *testing.T) {
	if _, err := ParseMoney("USD", "-1"); err == nil {
		t.Fatal("negative money accepted")
	}
	if _, err := ParseMoney("USD", "1.0000001"); err == nil {
		t.Fatal("excess precision accepted")
	}
	if _, err := ParseMoney("USD", "9223372036855.808000"); err == nil {
		t.Fatal("overflow money accepted")
	}
	if _, err := ParseMoney("ZZZ", "1"); err == nil {
		t.Fatal("unknown currency accepted")
	}
}

func TestMoneyOperationsRejectOverflow(t *testing.T) {
	left, err := NewMoney("USD", math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := left.Add(Money{Currency: "USD", Micros: 1}); err == nil {
		t.Fatal("addition overflow accepted")
	}
	if _, err := left.Multiply(2); err == nil {
		t.Fatal("multiplication overflow accepted")
	}
}

func TestTerminalStatesCannotTransition(t *testing.T) {
	if err := TransitionRequest(RequestSucceeded, RequestRunning); err == nil {
		t.Fatal("succeeded request became running")
	}
	if err := TransitionRequest(RequestPending, RequestRunning); err != nil {
		t.Fatal(err)
	}
	if err := TransitionAttempt(AttemptUnknown, AttemptRunning); err == nil {
		t.Fatal("unknown attempt became running")
	}
	if err := TransitionCredential(CredentialDisabled, CredentialActive); err == nil {
		t.Fatal("disabled credential became active")
	}
}

func TestUUIDv7AndScopedIDRoundTrip(t *testing.T) {
	id, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if id[6]>>4 != 7 || id[8]&0xc0 != 0x80 {
		t.Fatalf("not UUIDv7: %x", id)
	}
	parsed, err := ParseUUID(id.String())
	if err != nil || parsed != id {
		t.Fatalf("UUID round trip failed: %v", err)
	}
	req, err := ParseRequestID(id.String())
	if err != nil || req.String() != id.String() {
		t.Fatalf("request ID round trip failed: %v", err)
	}
}

func TestDomainRejectsMalformedDirectValues(t *testing.T) {
	if _, err := (Money{Currency: "USD", Micros: -1}).Add(Money{Currency: "USD"}); err == nil {
		t.Fatal("negative direct money accepted")
	}
	if err := (CapabilityDescriptor{Name: "bogus", Version: "1"}).Validate(); err == nil {
		t.Fatal("unknown capability accepted")
	}
	if err := (Usage{InputTokens: -1, Completeness: UsageComplete}).Validate(); err == nil {
		t.Fatal("negative usage accepted")
	}
}

func TestUnknownAttemptIsPersistableTerminalState(t *testing.T) {
	id, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	attempt := Attempt{ID: AttemptID(id), Request: RequestID(id), Number: 1, State: AttemptUnknown, CredentialVersion: 1}
	if err := attempt.Validate(); err != nil {
		t.Fatalf("unknown attempt must be persistable: %v", err)
	}
	if err := TransitionAttempt(AttemptUnknown, AttemptRunning); err == nil {
		t.Fatal("unknown attempt became runnable")
	}
}

func TestUsagePartialAndMissingSemantics(t *testing.T) {
	id, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	partial := Usage{Request: RequestID(id), Attempt: AttemptID(id), InputTokens: 100, InputKnown: true, OutputTokens: 50, OutputKnown: true, Completeness: UsagePartial, Source: UsageSourceProvider}
	if err := partial.Validate(); err != nil {
		t.Fatalf("valid partial usage rejected: %v", err)
	}
	missing := Usage{Request: RequestID(id), Attempt: AttemptID(id), Completeness: UsageMissing, Source: UsageSourceUnknown}
	if err := missing.Validate(); err != nil {
		t.Fatalf("valid missing usage rejected: %v", err)
	}
	complete := partial
	complete.Completeness = UsageComplete
	if err := complete.Validate(); err == nil {
		t.Fatal("incomplete usage accepted as complete")
	}
}
