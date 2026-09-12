// Package provider defines the small boundary between protocol handling and
// an authorized upstream adapter. It deliberately contains no HTTP client or
// listener implementation.
package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/starhui-dev/urbino/internal/protocol"
)

type BudgetInputs struct{ MaxInputBytes, MaxOutputTokens, MaxBatchItems int }
type RequestPolicy struct {
	MaxBodyBytes, MaxDepth int
	Budget                 BudgetInputs
}
type ValidatedRequest struct {
	Protocol, Model, Endpoint string
	Body                      json.RawMessage
	Budget                    BudgetInputs
}
type AuthorizedTarget struct {
	Origin, Path, Method string
	Headers              http.Header
	Credential           CredentialMaterial
}

// Secret is short lived material supplied by a vault. Adapters must not store
// it or log it; callers should clear it as soon as request construction ends.
type CredentialMaterial struct {
	Secret  []byte
	Version string
}
type ResponseClassification struct {
	Class      ResponseClass
	Dispatch   DispatchCertainty
	RetryAfter time.Duration
}
type ResponseClass uint8

const (
	ResponseSuccess ResponseClass = iota + 1
	ResponseUnauthorized
	ResponseForbidden
	ResponseRateLimited
	ResponseBadRequest
	ResponseUnavailable
	ResponseUnknown
)

type DispatchCertainty uint8

const (
	DispatchUnknown DispatchCertainty = iota
	DispatchNotAccepted
	DispatchAccepted
	DispatchCompleted
)

type Usage struct {
	InputTokens, OutputTokens, TotalTokens *int64
	Complete                               bool
	Source                                 string
	Raw                                    json.RawMessage
}

type Provider interface {
	Name() string
	Validate(context.Context, json.RawMessage, protocol.CapabilityDescriptor, RequestPolicy) (ValidatedRequest, error)
	BuildRequest(context.Context, AuthorizedTarget, ValidatedRequest) (*http.Request, error)
	ClassifyResponse(status int, headers http.Header, boundedBody []byte) ResponseClassification
	ExtractUsage(response json.RawMessage) (Usage, error)
	MapPublicError(error) protocol.PublicError
}

type StreamObserver interface {
	ObserveEvent(context.Context, []byte) error
}
