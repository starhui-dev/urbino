package domain

import "fmt"

type RequestState string

const (
	RequestPending   RequestState = "pending"
	RequestRunning   RequestState = "running"
	RequestSucceeded RequestState = "succeeded"
	RequestFailed    RequestState = "failed"
	RequestCancelled RequestState = "cancelled"
)

func (s RequestState) Valid() bool {
	switch s {
	case RequestPending, RequestRunning, RequestSucceeded, RequestFailed, RequestCancelled:
		return true
	default:
		return false
	}
}

func TransitionRequest(from, to RequestState) error {
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("invalid request state transition: %q -> %q", from, to)
	}
	allowed := (from == RequestPending && (to == RequestRunning || to == RequestCancelled || to == RequestFailed)) ||
		(from == RequestRunning && (to == RequestSucceeded || to == RequestFailed || to == RequestCancelled))
	if !allowed {
		return fmt.Errorf("invalid request state transition: %q -> %q", from, to)
	}
	return nil
}

type AttemptState string

const (
	AttemptPending   AttemptState = "pending"
	AttemptRunning   AttemptState = "running"
	AttemptSucceeded AttemptState = "succeeded"
	AttemptFailed    AttemptState = "failed"
	AttemptUnknown   AttemptState = "unknown"
)

func (s AttemptState) Valid() bool {
	switch s {
	case AttemptPending, AttemptRunning, AttemptSucceeded, AttemptFailed, AttemptUnknown:
		return true
	default:
		return false
	}
}

func TransitionAttempt(from, to AttemptState) error {
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("invalid attempt state transition: %q -> %q", from, to)
	}
	allowed := (from == AttemptPending && (to == AttemptRunning || to == AttemptFailed || to == AttemptUnknown)) ||
		(from == AttemptRunning && (to == AttemptSucceeded || to == AttemptFailed || to == AttemptUnknown))
	if !allowed {
		return fmt.Errorf("invalid attempt state transition: %q -> %q", from, to)
	}
	return nil
}

type CredentialState string

const (
	CredentialActive       CredentialState = "active"
	CredentialDisabled     CredentialState = "disabled"
	CredentialExpired      CredentialState = "expired"
	CredentialRequiresAuth CredentialState = "requires_reauth"
)

func (s CredentialState) Valid() bool {
	switch s {
	case CredentialActive, CredentialDisabled, CredentialExpired, CredentialRequiresAuth:
		return true
	default:
		return false
	}
}

func TransitionCredential(from, to CredentialState) error {
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("invalid credential state transition: %q -> %q", from, to)
	}
	if from == CredentialActive && (to == CredentialDisabled || to == CredentialExpired || to == CredentialRequiresAuth) {
		return nil
	}
	return fmt.Errorf("invalid credential state transition: %q -> %q", from, to)
}
