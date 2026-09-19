package ports

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/urbino/internal/domain"
)

func testCredentialTarget(t *testing.T, authMode string) (Target, CredentialMaterial) {
	t.Helper()
	id, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	tenant := domain.TenantID(id)
	project := domain.ProjectID(id)
	account := AccountID{Tenant: tenant, Project: project, ID: id}
	ref := CredentialRef{Tenant: tenant, Project: project, Account: account, Provider: "provider", AuthMode: authMode, Version: 1}
	target := Target{Tenant: tenant, Project: project, Provider: "provider", Model: "model", Account: account, Credential: ref, Origin: "https://example.com", Path: "/v1"}
	return target, CredentialMaterial{Ref: ref, Value: []byte("secret")}
}

func TestEgressProfileRequiresValidatedHTTPSOrigins(t *testing.T) {
	if _, err := (EgressProfile{Name: "x", TLSProfile: "default", AllowedOrigins: []string{"http://example.com"}}).Validate(); err == nil {
		t.Fatal("non-HTTPS origin accepted")
	}
	if _, err := (EgressProfile{Name: "x", TLSProfile: "default", AllowedOrigins: []string{"https://example.com/path"}}).Validate(); err == nil {
		t.Fatal("origin with path accepted")
	}
	if _, err := (EgressProfile{Name: "x", TLSProfile: "default", AllowedOrigins: []string{"https://example.com"}}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialReferenceAndOutboundRequestFailClosed(t *testing.T) {
	if err := (CredentialRef{Version: 1}).Validate(); err == nil {
		t.Fatal("unscoped credential reference accepted")
	}
	if err := (OutboundRequest{Method: "POST", Origin: "https://example.com", Path: "https://metadata"}).Validate(); err == nil {
		t.Fatal("absolute outbound path accepted")
	}
	if err := (OutboundRequest{Method: "POST", Origin: "https://example.com", Path: "/v1", Header: map[string][]string{"Authorization": {"secret"}}}).Validate(); err == nil {
		t.Fatal("sensitive outbound header accepted")
	}
	id, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	account := AccountID{Tenant: domain.TenantID(id), Project: domain.ProjectID(id), ID: id}
	ref := CredentialRef{
		Tenant: domain.TenantID(id), Project: domain.ProjectID(id), Account: account,
		Provider: "provider", AuthMode: "api_key", Version: 1,
	}
	if err := ref.Validate(); err != nil {
		t.Fatal(err)
	}
	profile, err := (EgressProfile{Name: "x", TLSProfile: "default", AllowedOrigins: []string{"https://example.com"}}).Validate()
	if err != nil {
		t.Fatal(err)
	}
	outboundRequest := OutboundRequest{Method: "POST", Origin: "https://other.example", Path: "/v1"}
	if _, err := profile.Bind(outboundRequest); err == nil {
		t.Fatal("unapproved outbound origin accepted")
	}
	for _, path := range []string{"/v1/%2e%2e/metadata", "/v1/..%2fmetadata", "/v1/%5cmetadata", "/v1/./metadata", "/v1/a/./b"} {
		if err := (OutboundRequest{Method: "POST", Origin: "https://example.com", Path: path}).Validate(); err == nil {
			t.Fatalf("path escape accepted: %s", path)
		}
	}
	payload := []byte("abc")
	validatedRequest, err := NewValidatedProviderRequest(payload, ProviderPolicy{MaxRequestBytes: 3})
	if err != nil {
		t.Fatal(err)
	}
	payload[0] = 'x'
	copyPayload := validatedRequest.Payload()
	copyPayload[1] = 'x'
	if string(validatedRequest.Payload()) != "abc" {
		t.Fatal("validated provider payload is mutable")
	}
}

func TestAuthenticatedRequestUsesBoundCredentialChannel(t *testing.T) {
	target, material := testCredentialTarget(t, "bearer")
	authenticated, err := NewAuthenticatedOutboundRequest(target, material, OutboundRequest{Method: http.MethodPost, Origin: target.Origin, Path: target.Path})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := mustEgressProfile(t).BindAuthenticated(authenticated)
	if err != nil {
		t.Fatal(err)
	}
	header := make(http.Header)
	if err := bound.ApplyCredential(header); err != nil {
		t.Fatal(err)
	}
	if got := header.Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("Authorization = %q", got)
	}
	if err := (OutboundRequest{Method: http.MethodPost, Origin: target.Origin, Path: target.Path, Header: http.Header{"Authorization": {"forged"}}}).Validate(); err == nil {
		t.Fatal("user-controlled authorization header accepted")
	}
}

func mustEgressProfile(t *testing.T) ValidatedEgressProfile {
	t.Helper()
	profile, err := (EgressProfile{Name: "x", TLSProfile: "default", AllowedOrigins: []string{"https://example.com"}}).Validate()
	if err != nil {
		t.Fatal(err)
	}
	return profile
}
func TestZeroCredentialOutboundBindRejected(t *testing.T) {
	if _, err := mustEgressProfile(t).Bind(OutboundRequest{Method: http.MethodPost, Origin: "https://example.com", Path: "/v1"}); err == nil {
		t.Fatal("zero-credential outbound request bound")
	}
}

func TestCredentialScopeMismatchRejected(t *testing.T) {
	target, material := testCredentialTarget(t, "api_key")
	mismatched := material
	mismatched.Ref.Provider = "other"
	if _, err := NewCredentialInjection(target, mismatched); err == nil {
		t.Fatal("mismatched credential material accepted")
	}
}

func TestBoundRequestSnapshotsHeaders(t *testing.T) {
	target, material := testCredentialTarget(t, "api_key")
	request := OutboundRequest{Method: http.MethodPost, Origin: target.Origin, Path: target.Path, Header: http.Header{"Content-Type": {"application/json"}}}
	authenticated, err := NewAuthenticatedOutboundRequest(target, material, request)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := mustEgressProfile(t).BindAuthenticated(authenticated)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "text/plain")
	if got := bound.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("bound header changed after source mutation: %q", got)
	}
}

func TestOutboundPathRejectsMultiEncodedEscapes(t *testing.T) {
	for _, path := range []string{"/v1/%252e%252e/metadata", "/v1/%252fmetadata", "/v1/%255cmetadata"} {
		if err := (OutboundRequest{Method: http.MethodPost, Origin: "https://example.com", Path: path}).Validate(); err == nil {
			t.Fatalf("multi-encoded path accepted: %s", path)
		}
	}
}

func TestUsageAndLeaseScopesAreRequired(t *testing.T) {
	target, _ := testCredentialTarget(t, "api_key")
	requestID, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	attemptID, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	scope := UsageContext{Tenant: target.Tenant, Project: target.Project, Request: domain.RequestID(requestID), Attempt: domain.AttemptID(attemptID)}
	usage := domain.Usage{Request: scope.Request, Attempt: scope.Attempt, InputKnown: true, OutputKnown: true, TotalKnown: true, Completeness: domain.UsageComplete, Source: domain.UsageSourceProvider}
	if err := scope.ValidateUsage(usage); err != nil {
		t.Fatal(err)
	}
	usage.Attempt = domain.AttemptID(requestID)
	if err := scope.ValidateUsage(usage); err == nil {
		t.Fatal("usage with mismatched attempt accepted")
	}
}

func TestBoundedResponseLimitsAreExplicit(t *testing.T) {
	body, err := NewBoundedBody(strings.NewReader("abcd"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(body.Reader()); !errors.Is(err, ErrBoundExceeded) {
		t.Fatalf("ReadAll error = %v, want ErrBoundExceeded", err)
	}
	if _, err := NewBoundedHeaders(http.Header{"X-Test": {strings.Repeat("x", maxHeaderValueBytes+1)}}, 2); err == nil {
		t.Fatal("oversized header value accepted")
	}
	if _, err := NewBoundedHeaders(http.Header{"": {"x"}}, 2); err == nil {
		t.Fatal("empty header name accepted")
	}
	if _, err := io.ReadAll(NewBoundedBodyMust(t, bytes.NewBufferString("ok"), 2).Reader()); err != nil {
		t.Fatal(err)
	}
}

func NewBoundedBodyMust(t *testing.T, reader io.Reader, limit int64) BoundedBody {
	t.Helper()
	body, err := NewBoundedBody(reader, limit)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
