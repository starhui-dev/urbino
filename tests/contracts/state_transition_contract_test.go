package contracts_test

// P01-T02 (checklists/test-matrix.csv: 状态转移): state advances only along
// the explicit transition tables. Terminal states (succeeded/failed/cancelled
// for requests, succeeded/failed/unknown for attempts, disabled/expired/
// requires_reauth for credentials) can never return to an executable state,
// terminal outcomes cannot be bypassed, and unknown state names are rejected.

import (
	"testing"

	"example.com/urbino/internal/domain"
)

func TestP01T02RequestTransitions(t *testing.T) {
	legal := []struct{ from, to domain.RequestState }{
		{domain.RequestPending, domain.RequestRunning},
		{domain.RequestPending, domain.RequestCancelled},
		{domain.RequestPending, domain.RequestFailed},
		{domain.RequestRunning, domain.RequestSucceeded},
		{domain.RequestRunning, domain.RequestFailed},
		{domain.RequestRunning, domain.RequestCancelled},
	}
	for _, tr := range legal {
		if err := domain.TransitionRequest(tr.from, tr.to); err != nil {
			t.Errorf("TransitionRequest(%q -> %q) unexpected error: %v", tr.from, tr.to, err)
		}
	}

	illegal := []struct {
		from, to domain.RequestState
		why      string
	}{
		{domain.RequestPending, domain.RequestSucceeded, "must not bypass the running state"},
		{domain.RequestRunning, domain.RequestPending, "must not rewind a running request"},
		{domain.RequestSucceeded, domain.RequestRunning, "completed must not return to executable"},
		{domain.RequestSucceeded, domain.RequestFailed, "terminal state must be closed"},
		{domain.RequestFailed, domain.RequestRunning, "failed must not return to executable"},
		{domain.RequestCancelled, domain.RequestPending, "cancelled must not restart"},
		{domain.RequestPending, domain.RequestPending, "self transition is not a table entry"},
		{domain.RequestState("bogus"), domain.RequestPending, "unknown source state"},
		{domain.RequestPending, domain.RequestState("bogus"), "unknown target state"},
	}
	for _, tr := range illegal {
		if err := domain.TransitionRequest(tr.from, tr.to); err == nil {
			t.Errorf("TransitionRequest(%q -> %q) accepted: %s", tr.from, tr.to, tr.why)
		}
	}
}

func TestP01T02AttemptTransitions(t *testing.T) {
	legal := []struct{ from, to domain.AttemptState }{
		{domain.AttemptPending, domain.AttemptRunning},
		{domain.AttemptPending, domain.AttemptFailed},
		{domain.AttemptRunning, domain.AttemptSucceeded},
		{domain.AttemptRunning, domain.AttemptFailed},
		{domain.AttemptRunning, domain.AttemptUnknown},
	}
	for _, tr := range legal {
		if err := domain.TransitionAttempt(tr.from, tr.to); err != nil {
			t.Errorf("TransitionAttempt(%q -> %q) unexpected error: %v", tr.from, tr.to, err)
		}
	}

	illegal := []struct {
		from, to domain.AttemptState
		why      string
	}{
		{domain.AttemptPending, domain.AttemptSucceeded, "must not bypass the running state"},
		{domain.AttemptRunning, domain.AttemptPending, "must not rewind a running attempt"},
		{domain.AttemptSucceeded, domain.AttemptRunning, "completed must not return to executable"},
		{domain.AttemptFailed, domain.AttemptRunning, "failed must not return to executable"},
		// unknown marks an uncertain upstream outcome: it must stay terminal so
		// uncertainty can never be laundered into a retry.
		{domain.AttemptUnknown, domain.AttemptRunning, "unknown must stay terminal"},
		{domain.AttemptUnknown, domain.AttemptPending, "unknown must stay terminal"},
		{domain.AttemptState("bogus"), domain.AttemptPending, "unknown source state"},
	}
	for _, tr := range illegal {
		if err := domain.TransitionAttempt(tr.from, tr.to); err == nil {
			t.Errorf("TransitionAttempt(%q -> %q) accepted: %s", tr.from, tr.to, tr.why)
		}
	}
}

func TestP01T02CredentialTransitions(t *testing.T) {
	legal := []struct{ from, to domain.CredentialState }{
		{domain.CredentialActive, domain.CredentialDisabled},
		{domain.CredentialActive, domain.CredentialExpired},
		{domain.CredentialActive, domain.CredentialRequiresAuth},
	}
	for _, tr := range legal {
		if err := domain.TransitionCredential(tr.from, tr.to); err != nil {
			t.Errorf("TransitionCredential(%q -> %q) unexpected error: %v", tr.from, tr.to, err)
		}
	}

	illegal := []struct {
		from, to domain.CredentialState
		why      string
	}{
		{domain.CredentialDisabled, domain.CredentialActive, "disabled must not be reactivated"},
		{domain.CredentialExpired, domain.CredentialActive, "expired must not be reactivated"},
		// requires_reauth must not loop back: re-auth issues a new version.
		{domain.CredentialRequiresAuth, domain.CredentialActive, "requires_reauth must not self-heal"},
		{domain.CredentialDisabled, domain.CredentialExpired, "disabled is terminal"},
		{domain.CredentialDisabled, domain.CredentialRequiresAuth, "disabled is terminal"},
		{domain.CredentialActive, domain.CredentialActive, "self transition is not a table entry"},
		{domain.CredentialState("bogus"), domain.CredentialActive, "unknown source state"},
	}
	for _, tr := range illegal {
		if err := domain.TransitionCredential(tr.from, tr.to); err == nil {
			t.Errorf("TransitionCredential(%q -> %q) accepted: %s", tr.from, tr.to, tr.why)
		}
	}
}
