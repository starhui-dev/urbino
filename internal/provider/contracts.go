package provider

import (
	"context"
	"net/http"
	"time"

	"github.com/starhui-dev/urbino/internal/domain"
)

// TransportProfile is a versioned, administrator supplied egress policy.
// Implementations must enforce origin, TLS and proxy policy before returning a client.
type TransportProfile struct {
	Origin    string
	TLSVerify bool
	ProxyURL  string
	Version   string
}

// TransportFactory is the only port for constructing outbound HTTP clients.
// Providers receive a client from this factory and never create one themselves.
type TransportFactory interface {
	NewClient(context.Context, TransportProfile) (*http.Client, error)
}

// Vault supplies short lived credential material for one exact credential version.
// Implementations must enforce account/tenant scope and must not persist or log Secret.
type Vault interface {
	ReadCredential(context.Context, domain.CredentialID, domain.CredentialVersion) (CredentialMaterial, error)
}

// UsageObserver receives typed usage observations and keeps accounting outside adapters.
type UsageObserver interface {
	ObserveUsage(context.Context, domain.RequestID, domain.AttemptID, Usage) error
}

// Reservation identifies a bounded admission hold owned by one request.
type ReservationRequest struct {
	RequestID domain.RequestID
	Amount    domain.Money
	ExpiresAt time.Time
}

// Reservation owns admission holds; implementations must make reserve/release idempotent.
type Reservation interface {
	Reserve(context.Context, ReservationRequest) error
	Release(context.Context, domain.RequestID) error
}

// ScheduleRequest contains only authorized, non-secret routing inputs.
type ScheduleRequest struct {
	Protocol string
	Model    string
}

// Scheduler selects an already authorized target; it does not perform network I/O.
type Scheduler interface {
	Select(context.Context, ScheduleRequest) (AuthorizedTarget, error)
}
