// Package ports contains narrow stage-one interfaces between domain workflows
// and later transport, vault, metering, quota and scheduling implementations.
package ports

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"example.com/urbino/internal/domain"
)

type TransportFactory interface {
	New(ctx context.Context, profile ValidatedEgressProfile) (OutboundTransport, error)
}

type EgressProfile struct {
	Name           string
	AllowedOrigins []string
	TLSProfile     string
}

type ValidatedEgressProfile struct {
	name          string
	allowedOrigin map[string]struct{}
	tlsProfile    string
}

func (p EgressProfile) Validate() (ValidatedEgressProfile, error) {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.TLSProfile) == "" || len(p.AllowedOrigins) == 0 {
		return ValidatedEgressProfile{}, fmt.Errorf("egress profile requires name, TLS profile, and allowed origins")
	}
	validated := ValidatedEgressProfile{name: p.Name, tlsProfile: p.TLSProfile, allowedOrigin: make(map[string]struct{}, len(p.AllowedOrigins))}
	for _, raw := range p.AllowedOrigins {
		origin, err := canonicalOrigin(raw)
		if err != nil {
			return ValidatedEgressProfile{}, err
		}
		validated.allowedOrigin[origin] = struct{}{}
	}
	return validated, nil
}

func (p ValidatedEgressProfile) AllowsOrigin(origin string) bool {
	canonical, err := canonicalOrigin(origin)
	if err != nil {
		return false
	}
	_, ok := p.allowedOrigin[canonical]
	return ok
}

type CredentialInjection struct {
	ref       CredentialRef
	value     []byte
	expiresAt domain.UTC
}

func (c CredentialInjection) IsZero() bool { return c.ref.Version == 0 && len(c.value) == 0 }

func (c CredentialInjection) Validate() error {
	if err := c.ref.Validate(); err != nil {
		return err
	}
	if len(c.value) == 0 {
		return errors.New("credential injection value is empty")
	}
	return nil
}

func (c CredentialInjection) Ref() CredentialRef { return c.ref }

func NewCredentialInjection(target Target, material CredentialMaterial) (CredentialInjection, error) {
	if err := target.Validate(); err != nil {
		return CredentialInjection{}, err
	}
	if err := material.Validate(); err != nil {
		return CredentialInjection{}, err
	}
	if material.Ref != target.Credential || material.Ref.Tenant != target.Tenant || material.Ref.Project != target.Project || material.Ref.Account != target.Account || material.Ref.Provider != target.Provider {
		return CredentialInjection{}, errors.New("credential material scope does not match target")
	}
	return CredentialInjection{ref: material.Ref, value: append([]byte(nil), material.Value...), expiresAt: material.ExpiresAt}, nil
}

func (c CredentialInjection) Apply(header http.Header) error {
	if err := c.Validate(); err != nil {
		return err
	}
	value := string(c.value)
	switch c.ref.AuthMode {
	case "api_key", "anthropic_api_key":
		header.Set("X-API-Key", value)
	case "bearer":
		header.Set("Authorization", "Bearer "+value)
	case "gemini_api_key":
		header.Set("X-Goog-Api-Key", value)
	default:
		return fmt.Errorf("unsupported credential auth mode %q", c.ref.AuthMode)
	}
	return nil
}

func (r OutboundRequest) WithCredential(injection CredentialInjection) OutboundRequest {
	r.Credential = injection
	return r
}

type OutboundRequest struct {
	Method     string
	Origin     string
	Path       string
	Header     http.Header
	Body       io.Reader
	Credential CredentialInjection
}

func (r OutboundRequest) Validate() error {
	if r.Method == "" || strings.IndexFunc(r.Method, func(r rune) bool { return r <= 0x20 || r >= 0x7f }) >= 0 {
		return fmt.Errorf("outbound request method is invalid")
	}
	if _, err := canonicalOrigin(r.Origin); err != nil {
		return err
	}
	u, err := url.ParseRequestURI(r.Path)
	decodedPath := r.Path
	for range 3 {
		next, unescapeErr := url.PathUnescape(decodedPath)
		if unescapeErr != nil {
			return fmt.Errorf("outbound request path is not a safe relative path")
		}
		if next == decodedPath {
			break
		}
		decodedPath = next
	}
	if err != nil || u.IsAbs() || !strings.HasPrefix(r.Path, "/") || u.RawQuery != "" || u.Fragment != "" || strings.Contains(strings.ToLower(r.Path), "%25") || strings.Contains(decodedPath, "%") || strings.Contains(decodedPath, "\\") || strings.IndexFunc(decodedPath, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return fmt.Errorf("outbound request path is not a safe relative path")
	}
	for _, segment := range strings.Split(decodedPath, "/") {
		if segment == "." || segment == ".." {
			return fmt.Errorf("outbound request path contains dot segment")
		}
	}
	if !r.Credential.IsZero() {
		if err := r.Credential.Validate(); err != nil {
			return err
		}
	}
	for key, values := range r.Header {
		canonicalKey := http.CanonicalHeaderKey(key)
		if canonicalKey == "" || canonicalKey != key || strings.IndexFunc(key, func(r rune) bool { return r <= 0x20 || r == 0x7f }) >= 0 {
			return fmt.Errorf("outbound request header name is invalid")
		}
		switch strings.ToLower(key) {
		case "accept", "accept-encoding", "content-type", "user-agent", "idempotency-key":
		default:
			return fmt.Errorf("outbound request header %q is not allowlisted", key)
		}
		for _, value := range values {
			if strings.IndexFunc(value, func(r rune) bool { return r == '\r' || r == '\n' || r < 0x20 || r == 0x7f }) >= 0 {
				return fmt.Errorf("outbound request header %q contains control characters", key)
			}
		}
	}
	return nil
}

type BoundOutboundRequest struct{ request OutboundRequest }

func (r BoundOutboundRequest) Method() string      { return r.request.Method }
func (r BoundOutboundRequest) Origin() string      { return r.request.Origin }
func (r BoundOutboundRequest) Path() string        { return r.request.Path }
func (r BoundOutboundRequest) Header() http.Header { return r.request.Header.Clone() }
func (r BoundOutboundRequest) Body() io.Reader     { return r.request.Body }
func (r BoundOutboundRequest) ApplyCredential(header http.Header) error {
	if r.request.Credential.IsZero() {
		return errors.New("bound outbound request has no credential injection")
	}
	return r.request.Credential.Apply(header)
}

func (p ValidatedEgressProfile) Bind(request OutboundRequest) (BoundOutboundRequest, error) {
	if request.Credential.IsZero() {
		return BoundOutboundRequest{}, errors.New("outbound request requires credential injection")
	}
	if err := request.Validate(); err != nil {
		return BoundOutboundRequest{}, err
	}
	if !p.AllowsOrigin(request.Origin) {
		return BoundOutboundRequest{}, fmt.Errorf("outbound origin is not allowed by egress profile")
	}
	request.Header = request.Header.Clone()
	return BoundOutboundRequest{request: request}, nil
}

func (p ValidatedEgressProfile) BindAuthenticated(request AuthenticatedOutboundRequest) (BoundOutboundRequest, error) {
	return p.Bind(request.request)
}

type OutboundTransport interface {
	Do(ctx context.Context, request BoundOutboundRequest) (*http.Response, error)
}

type AccountID struct {
	Tenant  domain.TenantID
	Project domain.ProjectID
	ID      domain.UUID
}

func (a AccountID) Validate() error {
	if a.Tenant.IsZero() || a.Project.IsZero() || a.ID.IsZero() {
		return fmt.Errorf("account identity is missing tenant/project scope")
	}
	return nil
}

type CredentialRef struct {
	Tenant   domain.TenantID
	Project  domain.ProjectID
	Account  AccountID
	Provider string
	AuthMode string
	Version  domain.CredentialVersion
}

func (r CredentialRef) Validate() error {
	if r.Tenant.IsZero() || r.Project.IsZero() || r.Provider == "" || r.AuthMode == "" || r.Version == 0 {
		return fmt.Errorf("credential reference is incomplete")
	}
	if err := r.Account.Validate(); err != nil {
		return err
	}
	if r.Account.Tenant != r.Tenant || r.Account.Project != r.Project {
		return fmt.Errorf("credential account scope does not match reference scope")
	}
	return nil
}

type Vault interface {
	Material(ctx context.Context, credential CredentialRef) (CredentialMaterial, error)
}

type CredentialMaterial struct {
	Ref       CredentialRef
	Value     []byte
	ExpiresAt domain.UTC
}

func (m CredentialMaterial) Validate() error {
	if err := m.Ref.Validate(); err != nil {
		return err
	}
	if len(m.Value) == 0 {
		return errors.New("credential material value is empty")
	}
	return nil
}

func (m CredentialMaterial) ValueCopy() []byte { return append([]byte(nil), m.Value...) }

type UsageContext struct {
	Tenant  domain.TenantID
	Project domain.ProjectID
	Request domain.RequestID
	Attempt domain.AttemptID
}

func (c UsageContext) Validate() error {
	if c.Tenant.IsZero() || c.Project.IsZero() || c.Request.IsZero() || c.Attempt.IsZero() {
		return errors.New("usage context is incomplete")
	}
	return nil
}

func (c UsageContext) ValidateUsage(usage domain.Usage) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if usage.Request != c.Request || usage.Attempt != c.Attempt {
		return errors.New("usage scope does not match context")
	}
	return usage.Validate()
}

type UsageObserver interface {
	Observe(ctx context.Context, scope UsageContext, usage domain.Usage) error
}

type Reservation interface {
	Acquire(ctx context.Context, scope UsageContext) (Lease, error)
	Release(ctx context.Context, lease Lease) error
}

type Lease struct {
	ID      domain.UUID
	Scope   UsageContext
	Owner   domain.UUID
	Fencing uint64
	Expires domain.UTC
}

func (l Lease) Validate() error {
	if l.ID.IsZero() || l.Owner.IsZero() || l.Fencing == 0 {
		return errors.New("lease identity is incomplete")
	}
	return l.Scope.Validate()
}

type Scheduler interface {
	Choose(ctx context.Context, request domain.Request) (Target, error)
}

type Target struct {
	Tenant     domain.TenantID
	Project    domain.ProjectID
	Provider   string
	Model      string
	Account    AccountID
	Credential CredentialRef
	Origin     string
	Path       string
}

func (t Target) Validate() error {
	if t.Tenant.IsZero() || t.Project.IsZero() || t.Provider == "" || t.Model == "" || t.Origin == "" || !strings.HasPrefix(t.Path, "/") {
		return fmt.Errorf("target is incomplete")
	}
	if err := t.Account.Validate(); err != nil {
		return err
	}
	if t.Account.Tenant != t.Tenant || t.Account.Project != t.Project {
		return fmt.Errorf("target account scope does not match target scope")
	}
	if err := t.Credential.Validate(); err != nil {
		return err
	}
	if t.Credential.Tenant != t.Tenant || t.Credential.Project != t.Project || t.Credential.Account != t.Account || t.Credential.Provider != t.Provider {
		return errors.New("target credential scope does not match target")
	}
	return (OutboundRequest{Method: http.MethodPost, Origin: t.Origin, Path: t.Path}).Validate()
}

type AuthenticatedOutboundRequest struct {
	target  Target
	request OutboundRequest
}

func NewAuthenticatedOutboundRequest(target Target, material CredentialMaterial, request OutboundRequest) (AuthenticatedOutboundRequest, error) {
	injection, err := NewCredentialInjection(target, material)
	if err != nil {
		return AuthenticatedOutboundRequest{}, err
	}
	if request.Origin != target.Origin || request.Path != target.Path {
		return AuthenticatedOutboundRequest{}, errors.New("provider request target does not match authenticated target")
	}
	request.Credential = injection
	if err := request.Validate(); err != nil {
		return AuthenticatedOutboundRequest{}, err
	}
	return AuthenticatedOutboundRequest{target: target, request: request}, nil
}

func (r AuthenticatedOutboundRequest) Request() OutboundRequest { return r.request }
func (r AuthenticatedOutboundRequest) Target() Target           { return r.target }

type ProviderPolicy struct{ MaxRequestBytes int64 }
type ValidatedProviderRequest struct {
	payload []byte
	budget  ProviderPolicy
}

func NewValidatedProviderRequest(payload []byte, policy ProviderPolicy) (ValidatedProviderRequest, error) {
	if policy.MaxRequestBytes <= 0 || int64(len(payload)) > policy.MaxRequestBytes {
		return ValidatedProviderRequest{}, fmt.Errorf("provider request exceeds configured budget")
	}
	copyPayload := append([]byte(nil), payload...)
	return ValidatedProviderRequest{payload: copyPayload, budget: policy}, nil
}

func (r ValidatedProviderRequest) Payload() []byte        { return append([]byte(nil), r.payload...) }
func (r ValidatedProviderRequest) Budget() ProviderPolicy { return r.budget }

type DispatchCertainty string

const (
	DispatchNotStarted DispatchCertainty = "not_started"
	DispatchAccepted   DispatchCertainty = "accepted"
	DispatchUnknown    DispatchCertainty = "unknown"
)

type CooldownScope string

const (
	CooldownNone     CooldownScope = "none"
	CooldownAccount  CooldownScope = "account"
	CooldownProvider CooldownScope = "provider"
)

var ErrBoundExceeded = errors.New("bounded response exceeded configured limit")

const (
	maxHeaderValuesPerKey = 16
	maxHeaderValueBytes   = 8192
	maxHeaderBytes        = 65536
)

type BoundedHeaders struct{ values http.Header }

func NewBoundedHeaders(values http.Header, maxEntries int) (BoundedHeaders, error) {
	if maxEntries <= 0 || len(values) > maxEntries {
		return BoundedHeaders{}, fmt.Errorf("response headers exceed configured bound")
	}
	total := 0
	copyValues := make(http.Header, len(values))
	for key, entries := range values {
		if key == "" || http.CanonicalHeaderKey(key) != key || len(entries) > maxHeaderValuesPerKey {
			return BoundedHeaders{}, fmt.Errorf("response header name or values are invalid")
		}
		copyValues[key] = append([]string(nil), entries...)
		for _, value := range entries {
			if len(value) > maxHeaderValueBytes || strings.IndexFunc(value, func(r rune) bool { return r == '\r' || r == '\n' || r < 0x20 || r == 0x7f }) >= 0 {
				return BoundedHeaders{}, fmt.Errorf("response header value exceeds configured bound")
			}
			total += len(key) + len(value)
			if total > maxHeaderBytes {
				return BoundedHeaders{}, fmt.Errorf("response headers exceed configured byte bound")
			}
		}
	}
	return BoundedHeaders{values: copyValues}, nil
}

func (h BoundedHeaders) Header() http.Header { return h.values.Clone() }

type BoundedBody struct{ reader io.Reader }

type boundedReader struct {
	reader    io.Reader
	remaining int64
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.reader.Read(extra[:])
		if n > 0 {
			return 0, ErrBoundExceeded
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func NewBoundedBody(reader io.Reader, maxBytes int64) (BoundedBody, error) {
	if reader == nil || maxBytes <= 0 {
		return BoundedBody{}, fmt.Errorf("bounded response body requires a positive limit")
	}
	return BoundedBody{reader: &boundedReader{reader: reader, remaining: maxBytes}}, nil
}

func (b BoundedBody) Reader() io.Reader { return b.reader }

type BoundedResponse struct {
	Status  int
	Headers BoundedHeaders
	Body    BoundedBody
}

type ResponseClassification struct {
	Class     ResponseClass
	Certainty DispatchCertainty
	Cooldown  CooldownScope
	Retryable bool
}

type ProviderAdapter interface {
	Validate(ctx context.Context, request []byte, capability domain.CapabilityDescriptor, policy ProviderPolicy) (ValidatedProviderRequest, error)
	BuildRequest(ctx context.Context, target Target, material CredentialMaterial, request ValidatedProviderRequest) (AuthenticatedOutboundRequest, error)
	ClassifyResponse(ctx context.Context, response BoundedResponse) (ResponseClassification, error)
	ExtractUsage(ctx context.Context, scope UsageContext, body BoundedBody) (domain.Usage, error)
	MapPublicError(ctx context.Context, classification ResponseClassification) *domain.Error
	ObserveStream(ctx context.Context, scope UsageContext, body BoundedBody, observer UsageObserver) error
}

type ResponseClass string

const (
	ResponseSuccess       ResponseClass = "success"
	ResponseUnauthorized  ResponseClass = "unauthorized"
	ResponseUnavailable   ResponseClass = "unavailable"
	ResponseResultUnknown ResponseClass = "result_unknown"
)

func canonicalOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid HTTPS origin %q", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}
