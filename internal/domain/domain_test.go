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
