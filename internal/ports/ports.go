// Package ports contains narrow stage-one interfaces between domain workflows
// and later transport, vault, metering, quota and scheduling implementations.
package ports

import (
	"context"
	"io"
	"net/http"

	"example.com/urbino/internal/domain"
)

// TransportFactory creates an outbound transport under one centrally enforced
// egress policy. Providers must not instantiate a default http.Client.
type TransportFactory interface {
	New(ctx context.Context, profile EgressProfile) (OutboundTransport, error)
}

type EgressProfile struct {
	Name         string
	AllowedHosts []string
	TLSProfile   string
}

type OutboundTransport interface {
	Do(ctx context.Context, request *http.Request) (*http.Response, error)
}

// Vault returns short-lived credential material; callers cannot persist it.
type Vault interface {
	Material(ctx context.Context, credential domain.CredentialVersion) (CredentialMaterial, error)
}

type CredentialMaterial struct {
	Value     []byte
	ExpiresAt domain.UTC
}

// UsageObserver only observes typed usage and does not calculate charges.
type UsageObserver interface {
	Observe(ctx context.Context, request domain.RequestID, usage domain.Usage) error
}

// Reservation owns bounded admission leases, not billing settlement.
type Reservation interface {
	Acquire(ctx context.Context, request domain.RequestID) (Lease, error)
	Release(ctx context.Context, lease Lease) error
}

type Lease struct {
	ID      domain.UUID
	Expires domain.UTC
}

// Scheduler chooses an authorized target without owning credentials or money.
type Scheduler interface {
	Choose(ctx context.Context, request domain.Request) (Target, error)
}

type Target struct {
	Provider string
	Model    string
	Account  domain.UUID
}

// ProviderAdapter is the narrow protocol boundary. It builds/classifies/decodes;
// it does not perform charging or bypass the shared transport.
type ProviderAdapter interface {
	Validate(ctx context.Context, request []byte, capability domain.CapabilityDescriptor) error
	BuildRequest(ctx context.Context, target Target, material CredentialMaterial, request []byte) (*http.Request, error)
	ClassifyResponse(ctx context.Context, status int, headers http.Header, body io.Reader) (ResponseClass, error)
	ObserveStream(ctx context.Context, request domain.RequestID, body io.Reader, observer UsageObserver) error
}

type ResponseClass string

const (
	ResponseSuccess       ResponseClass = "success"
	ResponseUnauthorized  ResponseClass = "unauthorized"
	ResponseUnavailable   ResponseClass = "unavailable"
	ResponseResultUnknown ResponseClass = "result_unknown"
)
