package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/domain"
	"example.com/urbino/internal/ports"
	"example.com/urbino/internal/storage/postgres"
)

// AdminAPIMountPath is the subtree an administrator listener mounts the
// management API on. The routes authenticate themselves, so the listener must
// not wrap them in a second credential layer.
const AdminAPIMountPath = "/admin/v1/"

// adminCapabilityVersion is the version reported for served capabilities.
const adminCapabilityVersion = "0.1.0"

// maxAdminBodyBytes bounds every management request body.
const maxAdminBodyBytes = 64 << 10

// AdminStore is the persistence surface of the management API. The production
// implementation is *postgres.IdentityStore. Every mutation writes its audit
// event in the same transaction as the state change, so the API never has to
// append audit records itself. The scoped failures postgres.ErrNotFound,
// postgres.ErrVersionConflict and postgres.ErrMembershipRequired are mapped to
// stable management responses.
type AdminStore interface {
	ports.AdminMutationExecutor

	ListTenantsPage(ctx context.Context, limit int, cursor string) ([]domain.Tenant, string, error)
	GetTenant(ctx context.Context, id domain.TenantID) (domain.Tenant, error)
	ListAPIKeysAt(ctx context.Context, tenant domain.TenantID, project domain.ProjectID, status string, limit int, now time.Time) ([]auth.KeyRecord, error)
}

type requestBodyDigestKey struct{}

func requestBodyDigest(r *http.Request) []byte {
	if digest, ok := r.Context().Value(requestBodyDigestKey{}).([]byte); ok {
		return digest
	}
	digest := sha256.Sum256(nil)
	return digest[:]
}

func mutationKeyDigest(r *http.Request) []byte {
	digest := sha256.Sum256([]byte(strings.TrimSpace(r.Header.Get("Idempotency-Key"))))
	return digest[:]
}

func (a *AdminAPI) runMutation(w http.ResponseWriter, r *http.Request, principal auth.AdminPrincipal, operation string, fn func(ports.AdminMutationTx) (ports.AdminMutationOutcome, error)) bool {
	keyDigest := mutationKeyDigest(r)
	digestInput := []byte(r.Method + " " + r.URL.RequestURI() + " ")
	digestInput = append(digestInput, requestBodyDigest(r)...)
	digestInput = append(digestInput, []byte("\x00if-match:"+strings.TrimSpace(r.Header.Get("If-Match")))...)
	requestDigest := sha256.Sum256(digestInput)
	result, err := a.store.RunAdminMutation(r.Context(), ports.AdminMutationRequest{
		Principal: principal.AdminID, Operation: operation, KeyDigest: keyDigest, RequestDigest: requestDigest[:],
	}, fn)
	if err != nil {
		writeStoreError(w, requestIDFrom(r.Context()), err)
		return false
	}
	writeAdminResponse(w, result.Response)
	return true
}

func adminMutationResponse(status int, value any) (ports.AdminMutationResponse, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return ports.AdminMutationResponse{}, err
	}
	return ports.AdminMutationResponse{Status: status, ContentType: "application/json", Body: body}, nil
}

func writeAdminResponse(w http.ResponseWriter, response ports.AdminMutationResponse) {
	w.Header().Set("Content-Type", response.ContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(response.Status)
	_, _ = w.Write(response.Body)
}

// AdminDeps wires the management API. Store and AuthenticateAdmin are
// required; Authorize defaults to AuthorizeAdminScope, Limiter to a fresh
// failure limiter, and Clock to time.Now.
type AdminDeps struct {
	Store             AdminStore
	Issuer            auth.Issuer
	AuthenticateAdmin func(context.Context, auth.Request) (auth.AdminPrincipal, error)
	// Authorize enforces administrator RBAC. Pass
	// (*auth.Service).CheckAdminScope when a service is wired; the PostgreSQL
	// authenticator path uses the AuthorizeAdminScope default.
	Authorize func(auth.AdminPrincipal, string) error
	Limiter   *auth.FailureLimiter
	Clock     func() time.Time
}

// AdminAPI is the minimal management surface: it serves the capability
// descriptor, tenant management, public key issuance/listing/revocation and
// administrator token revocation on the administrator listener. It never
// returns a secret it did not just mint, and it never returns key digests.
type AdminAPI struct {
	store             AdminStore
	issuer            auth.Issuer
	authenticateAdmin func(context.Context, auth.Request) (auth.AdminPrincipal, error)
	authorize         func(auth.AdminPrincipal, string) error
	limiter           *auth.FailureLimiter
	now               func() time.Time
}

// NewAdminAPI validates its dependencies and returns the management API. It
// fails closed: a missing store, authenticator or usable issuer pepper is a
// construction error, never a request-time 500.
func NewAdminAPI(deps AdminDeps) (*AdminAPI, error) {
	if deps.Store == nil {
		return nil, errors.New("httpapi: management store is required")
	}
	if deps.AuthenticateAdmin == nil {
		return nil, errors.New("httpapi: administrator authentication is required")
	}
	if deps.Issuer.Current.ID == "" || len(deps.Issuer.Current.Key) < 32 {
		return nil, errors.New("httpapi: management issuer pepper is invalid")
	}
	authorize := deps.Authorize
	if authorize == nil {
		authorize = AuthorizeAdminScope
	}
	limiter := deps.Limiter
	if limiter == nil {
		created, err := auth.NewFailureLimiter(10, time.Minute)
		if err != nil {
			return nil, err
		}
		limiter = created
	}
	now := deps.Clock
	if now == nil {
		now = time.Now
	}
	return &AdminAPI{
		store: deps.Store, issuer: deps.Issuer, authenticateAdmin: deps.AuthenticateAdmin,
		authorize: authorize, limiter: limiter, now: now,
	}, nil
}

// AuthorizeAdminScope is the default RBAC check for the PostgreSQL
// authenticator path, where no *auth.Service exists. It mirrors
// (*auth.Service).CheckAdminScope and denies every unknown scope.
func AuthorizeAdminScope(principal auth.AdminPrincipal, scope string) error {
	for _, candidate := range principal.Scopes {
		if candidate == scope {
			return nil
		}
	}
	return auth.ErrScopeDenied
}

// Handler returns the authenticated management routes.
func (a *AdminAPI) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/v1/capabilities", a.require("", a.handleCapabilities))
	mux.HandleFunc("GET /admin/v1/tenants", a.require(domain.ScopeTenantsRead, a.handleListTenants))
	mux.HandleFunc("POST /admin/v1/tenants", a.require(domain.ScopeTenantsWrite, a.handleCreateTenant))
	mux.HandleFunc("GET /admin/v1/tenants/{tenantId}", a.require(domain.ScopeTenantsRead, a.handleGetTenant))
	mux.HandleFunc("PATCH /admin/v1/tenants/{tenantId}", a.require(domain.ScopeTenantsWrite, a.handleUpdateTenant))
	mux.HandleFunc("GET /admin/v1/api-keys", a.require(domain.ScopeKeysRead, a.handleListAPIKeys))
	mux.HandleFunc("POST /admin/v1/api-keys", a.require(domain.ScopeKeysWrite, a.handleCreateAPIKey))
	mux.HandleFunc("POST /admin/v1/api-keys/{publicId}/revoke", a.require(domain.ScopeKeysWrite, a.handleRevokeAPIKey))
	mux.HandleFunc("POST /admin/v1/admin-tokens/{publicId}/revoke", a.require(domain.ScopeKeysWrite, a.handleRevokeAdminToken))
	return CredentialMiddleware{
		Mode:              auth.AdminListener,
		Limiter:           a.limiter,
		AuthenticateAdmin: a.authenticateAdmin,
	}.Wrap(mux)
}

// Mount registers the management routes on an administrator listener mux.
func (a *AdminAPI) Mount(mux *http.ServeMux) {
	if a == nil || mux == nil {
		return
	}
	mux.Handle(AdminAPIMountPath, a.Handler())
}

// require authenticates nothing itself: CredentialMiddleware already resolved
// the administrator principal, and this layer only enforces RBAC.
func (a *AdminAPI) require(scope domain.PermissionScope, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := requestIDFrom(r.Context())
		if requestID.IsZero() {
			requestID = newRequestID()
		}
		if r.Method != http.MethodGet && strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
			writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "Idempotency-Key is required for management mutations")
			return
		}
		principal, ok := AdminPrincipal(r.Context())
		if !ok {
			writeAdminError(w, requestID, http.StatusUnauthorized, domain.CodeUnauthorized, "administrator credential required")
			return
		}
		if scope != "" {
			if err := a.authorize(principal, string(scope)); err != nil {
				writeAdminError(w, requestID, http.StatusForbidden, domain.CodePermissionDenied, "administrator scope does not permit this operation")
				return
			}
		}
		next(w, r.WithContext(context.WithValue(r.Context(), adminRequestIDKey{}, requestID)))
	}
}

func (a *AdminAPI) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, capabilityDescriptors())
}

func (a *AdminAPI) handleListTenants(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r.Context())
	limit, err := parseLimit(r.URL.Query())
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid limit")
		return
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	tenants, nextCursor, err := a.store.ListTenantsPage(r.Context(), limit, cursor)
	if err != nil {
		if strings.Contains(err.Error(), "invalid tenant cursor") {
			writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid cursor")
		} else {
			writeStoreError(w, requestID, err)
		}
		return
	}
	var next *string
	if nextCursor != "" {
		next = &nextCursor
	}
	page := tenantPage{Items: make([]tenantResponse, 0, len(tenants)), NextCursor: next}
	for _, tenant := range tenants {
		page.Items = append(page.Items, tenantView(tenant))
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *AdminAPI) handleGetTenant(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r.Context())
	tenantID, err := domain.ParseTenantID(r.PathValue("tenantId"))
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid tenant identifier")
		return
	}
	tenant, err := a.store.GetTenant(r.Context(), tenantID)
	if err != nil {
		writeStoreError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, tenantView(tenant))
}

func (a *AdminAPI) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r.Context())
	var body createTenantRequest
	if !decodeAdminBody(w, r, requestID, &body) {
		return
	}
	tenantID, err := domain.NewUUID()
	if err != nil {
		writeAdminError(w, requestID, http.StatusInternalServerError, domain.CodeUnknown, "management operation failed")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 200 {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid tenant name")
		return
	}
	tenant := domain.Tenant{
		ID:       domain.TenantID(tenantID),
		Name:     name,
		Currency: domain.Currency(body.Currency),
		Status:   domain.TenantActive,
		Version:  1,
	}
	if err := tenant.Validate(); err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid tenant")
		return
	}
	principal, _ := AdminPrincipal(r.Context())
	a.runMutation(w, r, principal, "tenant.create", func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
		if err := tx.CreateTenantFor(r.Context(), principal.AdminID, tenant); err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		response, err := adminMutationResponse(http.StatusCreated, tenantView(tenant))
		if err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		return ports.AdminMutationOutcome{Response: response, ReplayResponse: response}, nil
	})
}

func (a *AdminAPI) handleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r.Context())
	tenantID, err := domain.ParseTenantID(r.PathValue("tenantId"))
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid tenant identifier")
		return
	}
	expectedVersion, err := parseIfMatchVersion(r.Header.Get("If-Match"))
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "If-Match must carry the expected tenant version")
		return
	}
	var body updateTenantRequest
	if !decodeAdminBody(w, r, requestID, &body) {
		return
	}
	if body.Name == nil && body.Status == nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "at least one of name or status is required")
		return
	}
	current, err := a.store.GetTenant(r.Context(), tenantID)
	if err != nil {
		writeStoreError(w, requestID, err)
		return
	}
	name := current.Name
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
		if name == "" || len(name) > 200 {
			writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid tenant name")
			return
		}
	}
	status := current.Status
	if body.Status != nil {
		status = domain.TenantStatus(*body.Status)
		if !status.Valid() {
			writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid tenant status")
			return
		}
	}
	principal, _ := AdminPrincipal(r.Context())
	a.runMutation(w, r, principal, "tenant.update", func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
		updated, err := tx.UpdateTenantFor(r.Context(), principal.AdminID, tenantID, expectedVersion, name, status)
		if err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		response, err := adminMutationResponse(http.StatusOK, tenantView(updated))
		if err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		return ports.AdminMutationOutcome{Response: response, ReplayResponse: response}, nil
	})
}

func (a *AdminAPI) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r.Context())
	query := r.URL.Query()
	tenantID, projectID, ok := scopeFromQuery(query)
	if !ok {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "tenant_id and project_id are required")
		return
	}
	status := query.Get("status")
	switch status {
	case "", "active", "revoked", "expired":
	default:
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid key status filter")
		return
	}
	limit, err := parseLimit(query)
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid limit")
		return
	}
	records, err := a.store.ListAPIKeysAt(r.Context(), tenantID, projectID, status, limit, a.now().UTC())
	if err != nil {
		writeStoreError(w, requestID, err)
		return
	}
	page := apiKeyPage{Items: make([]apiKeyResponse, 0, len(records))}
	for _, record := range records {
		page.Items = append(page.Items, apiKeyView(record))
	}
	writeJSON(w, http.StatusOK, page)
}
func (a *AdminAPI) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r.Context())
	var body createAPIKeyRequest
	if !decodeAdminBody(w, r, requestID, &body) {
		return
	}
	tenantID, err := domain.ParseTenantID(body.TenantID)
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid tenant_id")
		return
	}
	projectID, err := domain.ParseProjectID(body.ProjectID)
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid project_id")
		return
	}
	userID, err := domain.ParsePrincipalID(body.UserID)
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid user_id")
		return
	}
	scopes, err := keyScopes(body.Scopes)
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, err.Error())
		return
	}
	models, err := keyModels(body.Models)
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, err.Error())
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(body.ExpiresAt))
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "expires_at must be an RFC 3339 timestamp")
		return
	}
	principal, _ := AdminPrincipal(r.Context())
	a.runMutation(w, r, principal, "api_key.create", func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
		now := a.now().UTC()
		if !expiresAt.After(now) {
			return ports.AdminMutationOutcome{}, errors.New("expires_at must be in the future")
		}
		issued, err := a.issuer.Issue(tenantID, projectID, userID, scopes, models, now, expiresAt.Sub(now))
		if err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		if err := tx.CreateAPIKeyFor(r.Context(), principal.AdminID, issued); err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		first, err := adminMutationResponse(http.StatusCreated, createAPIKeyResponse{Key: issued.Value, apiKeyResponse: apiKeyView(issued.Record)})
		if err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		replay, err := adminMutationResponse(http.StatusCreated, createAPIKeyReplayResponse{
			apiKeyResponse:  apiKeyView(issued.Record),
			Message:         "the one-time secret is no longer available; rotate the key to obtain a new secret",
			SecretAvailable: false,
		})
		if err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		return ports.AdminMutationOutcome{Response: first, ReplayResponse: replay}, nil
	})
}

func (a *AdminAPI) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r.Context())
	publicID := strings.TrimSpace(r.PathValue("publicId"))
	if publicID == "" {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid key identifier")
		return
	}
	var body revokeAPIKeyRequest
	if !decodeAdminBody(w, r, requestID, &body) {
		return
	}
	tenantID, err := domain.ParseTenantID(body.TenantID)
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid tenant_id")
		return
	}
	projectID, err := domain.ParseProjectID(body.ProjectID)
	if err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid project_id")
		return
	}
	principal, _ := AdminPrincipal(r.Context())
	a.runMutation(w, r, principal, "api_key.revoke", func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
		revoked, err := tx.RevokeAPIKeyFor(r.Context(), principal.AdminID, tenantID, projectID, publicID, a.now().UTC())
		if err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		response, err := adminMutationResponse(http.StatusOK, apiKeyView(revoked))
		if err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		return ports.AdminMutationOutcome{Response: response, ReplayResponse: response}, nil
	})
}

// handleRevokeAdminToken revokes an administrator token of the calling
// principal only: revoking another principal's token needs a separate
// privilege and is intentionally outside this minimal surface.
func (a *AdminAPI) handleRevokeAdminToken(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r.Context())
	publicID := strings.TrimSpace(r.PathValue("publicId"))
	if publicID == "" {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "invalid token identifier")
		return
	}
	principal, ok := AdminPrincipal(r.Context())
	if !ok {
		writeAdminError(w, requestID, http.StatusUnauthorized, domain.CodeUnauthorized, "administrator credential required")
		return
	}
	a.runMutation(w, r, principal, "admin_token.revoke", func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
		revoked, err := tx.RevokeAdminTokenFor(r.Context(), principal.AdminID, principal.AdminID, publicID, a.now().UTC())
		if err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		response, err := adminMutationResponse(http.StatusOK, adminTokenView(revoked))
		if err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		return ports.AdminMutationOutcome{Response: response, ReplayResponse: response}, nil
	})
}

// capabilityDescriptors reports what this deployment serves. The inference
// capabilities stay explicitly disabled so a caller never treats an
// unsupported route as available.
func capabilityDescriptors() []capabilityResponse {
	scopes := []string{
		string(domain.ScopeTenantsRead), string(domain.ScopeTenantsWrite),
		string(domain.ScopeKeysRead), string(domain.ScopeKeysWrite),
		string(domain.ScopeModelsRead), string(domain.ScopeModelsInvoke), string(domain.ScopeUsageRead),
	}
	return []capabilityResponse{
		{Name: string(domain.CapabilityAdminAPI), Version: adminCapabilityVersion, Enabled: true, Scopes: scopes},
		{Name: string(domain.CapabilityChat), Version: adminCapabilityVersion, Enabled: false, Scopes: []string{}},
		{Name: string(domain.CapabilityResponses), Version: adminCapabilityVersion, Enabled: false, Scopes: []string{}},
		{Name: string(domain.CapabilityEmbeddings), Version: adminCapabilityVersion, Enabled: false, Scopes: []string{}},
	}
}

func tenantView(tenant domain.Tenant) tenantResponse {
	return tenantResponse{
		ID:       tenant.ID.String(),
		Name:     tenant.Name,
		Currency: string(tenant.Currency),
		Status:   string(tenant.Status),
		Version:  tenant.Version,
	}
}

// apiKeyView renders key metadata only: the plaintext secret and the stored
// digest never leave the storage boundary.
func apiKeyView(record auth.KeyRecord) apiKeyResponse {
	return apiKeyResponse{
		PublicID:    record.PublicID,
		TenantID:    record.Tenant.String(),
		ProjectID:   record.Project.String(),
		UserID:      record.User.String(),
		Scopes:      append([]string{}, record.Scopes...),
		Models:      append([]string{}, record.Models...),
		Status:      record.Status,
		ExpiresAt:   record.ExpiresAt.UTC(),
		RevokedAt:   record.RevokedAt,
		AuthVersion: record.AuthVersion,
	}
}

func adminTokenView(record auth.AdminTokenRecord) adminTokenRevokeResponse {
	return adminTokenRevokeResponse{
		PublicID:    record.PublicID,
		PrincipalID: record.PrincipalID.String(),
		Status:      "revoked",
		RevokedAt:   record.RevokedAt,
		AuthVersion: record.AuthVersion,
	}
}

// keyScopes accepts only inference scopes: a gateway key must never carry
// management scopes.
func keyScopes(values []string) ([]domain.PermissionScope, error) {
	if len(values) == 0 {
		return nil, errors.New("scopes must not be empty")
	}
	scopes := make([]domain.PermissionScope, 0, len(values))
	for _, value := range values {
		scope := domain.PermissionScope(value)
		switch scope {
		case domain.ScopeModelsRead, domain.ScopeModelsInvoke, domain.ScopeUsageRead:
		default:
			return nil, errors.New("scope is not grantable to a gateway key")
		}
		scopes = append(scopes, scope)
	}
	return scopes, nil
}

func keyModels(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("models must not be empty")
	}
	models := make([]string, 0, len(values))
	for _, value := range values {
		model := strings.TrimSpace(value)
		if model == "" || len(model) > 200 {
			return nil, errors.New("model name is invalid")
		}
		models = append(models, model)
	}
	return models, nil
}

func scopeFromQuery(query url.Values) (domain.TenantID, domain.ProjectID, bool) {
	tenantID, err := domain.ParseTenantID(strings.TrimSpace(query.Get("tenant_id")))
	if err != nil {
		return domain.TenantID{}, domain.ProjectID{}, false
	}
	projectID, err := domain.ParseProjectID(strings.TrimSpace(query.Get("project_id")))
	if err != nil {
		return domain.TenantID{}, domain.ProjectID{}, false
	}
	return tenantID, projectID, true
}

func parseLimit(query url.Values) (int, error) {
	raw := strings.TrimSpace(query.Get("limit"))
	if raw == "" {
		return 50, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		return 0, errors.New("limit out of range")
	}
	return limit, nil
}

// parseIfMatchVersion accepts a bare or quoted entity version and rejects
// everything else, so an optimistic update can never silently ignore its
// precondition.
func parseIfMatchVersion(value string) (uint64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, errors.New("If-Match is required")
	}
	trimmed = strings.TrimPrefix(trimmed, "W/")
	trimmed = strings.Trim(trimmed, `"`)
	version, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil || version == 0 {
		return 0, errors.New("If-Match is invalid")
	}
	return version, nil
}

// decodeAdminBody enforces a bounded JSON body with an explicit JSON content
// type, no unknown fields and no duplicate object keys.
func decodeAdminBody(w http.ResponseWriter, r *http.Request, requestID domain.UUID, dst any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeAdminError(w, requestID, http.StatusUnsupportedMediaType, domain.CodeBadRequest, "Content-Type must be application/json")
		return false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAdminBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeAdminError(w, requestID, http.StatusRequestEntityTooLarge, domain.CodeBadRequest, "request body is too large")
			return false
		}
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "request body is unreadable")
		return false
	}
	digest := sha256.Sum256(raw)
	*r = *r.WithContext(context.WithValue(r.Context(), requestBodyDigestKey{}, digest[:]))
	if err := checkDuplicateJSONKeys(raw); err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "request body is invalid JSON")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "request body is invalid JSON")
		return false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		writeAdminError(w, requestID, http.StatusBadRequest, domain.CodeBadRequest, "request body is invalid JSON")
		return false
	}
	return true
}

// checkDuplicateJSONKeys walks the raw body so a duplicate object key is a
// rejection instead of a silent last-writer-wins decode.
func checkDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	return scanJSONValue(decoder, 0)
}

func scanJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("json nesting is too deep")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("json object key is invalid")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("json object key is duplicated")
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	}
	_, err = decoder.Token()
	return err
}

type adminRequestIDKey struct{}

func requestIDFrom(ctx context.Context) domain.UUID {
	requestID, ok := ctx.Value(adminRequestIDKey{}).(domain.UUID)
	if !ok {
		return domain.UUID{}
	}
	return requestID
}

func newRequestID() domain.UUID {
	requestID, err := domain.NewUUID()
	if err != nil {
		return domain.UUID{}
	}
	return requestID
}

func writeAdminError(w http.ResponseWriter, requestID domain.UUID, status int, code domain.Code, message string) {
	writeJSON(w, status, publicError{
		Code: code, Message: message, RequestID: requestID.String(), Retryable: false, Details: map[string]string{},
	})
}

// writeStoreError maps a storage failure onto one stable management response
// without disclosing driver or scope details.
func writeStoreError(w http.ResponseWriter, requestID domain.UUID, err error) {
	status, code, message := adminErrorStatus(err)
	writeAdminError(w, requestID, status, code, message)
}

func adminErrorStatus(err error) (int, domain.Code, string) {
	switch {
	case errors.Is(err, postgres.ErrIdempotencyConflict), errors.Is(err, postgres.ErrIdempotencyPayloadConflict):
		return http.StatusConflict, domain.CodeVersionConflict, "Idempotency-Key was already used"
	case errors.Is(err, postgres.ErrNotFound):
		return http.StatusNotFound, domain.CodeResourceNotFound, "resource not found"
	case errors.Is(err, postgres.ErrVersionConflict):
		return http.StatusPreconditionFailed, domain.CodeVersionConflict, "the resource version does not match the precondition"
	case errors.Is(err, postgres.ErrMembershipRequired):
		return http.StatusBadRequest, domain.CodeBadRequest, "the user is not an active member of the project"
	case errors.Is(err, postgres.ErrCommitOutcomeUnknown):
		return http.StatusServiceUnavailable, domain.CodeUnknown, "management operation outcome is unknown; do not retry blindly"
	case errors.Is(err, postgres.ErrLastAdminToken):
		return http.StatusConflict, domain.CodeVersionConflict, "the final usable administrator token cannot be revoked"
	}
	var dbErr *postgres.Error
	if errors.As(err, &dbErr) {
		switch {
		case dbErr.IsKind(postgres.ErrorConflict):
			return http.StatusConflict, domain.CodeVersionConflict, "the resource conflicts with existing state"
		case dbErr.IsKind(postgres.ErrorUnavailable), dbErr.IsKind(postgres.ErrorCanceled):
			return http.StatusServiceUnavailable, domain.CodeUnknown, "the database is unavailable"
		}
	}
	return http.StatusInternalServerError, domain.CodeUnknown, "management operation failed"
}

type capabilityResponse struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Enabled bool     `json:"enabled"`
	Scopes  []string `json:"scopes"`
}

type createTenantRequest struct {
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

type updateTenantRequest struct {
	Name   *string `json:"name"`
	Status *string `json:"status"`
}

type tenantResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
	Version  uint64 `json:"version"`
}

type tenantPage struct {
	Items      []tenantResponse `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

type createAPIKeyRequest struct {
	TenantID  string   `json:"tenant_id"`
	ProjectID string   `json:"project_id"`
	UserID    string   `json:"user_id"`
	Scopes    []string `json:"scopes"`
	Models    []string `json:"models"`
	ExpiresAt string   `json:"expires_at"`
}

type revokeAPIKeyRequest struct {
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id"`
}

type apiKeyResponse struct {
	PublicID    string     `json:"public_id"`
	TenantID    string     `json:"tenant_id"`
	ProjectID   string     `json:"project_id"`
	UserID      string     `json:"user_id"`
	Scopes      []string   `json:"scopes"`
	Models      []string   `json:"models"`
	Status      string     `json:"status"`
	ExpiresAt   time.Time  `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
	AuthVersion uint64     `json:"auth_version"`
}

// createAPIKeyResponse is the only response that carries a credential, and it
// is served exactly once, at creation.
type createAPIKeyResponse struct {
	Key string `json:"key"`
	apiKeyResponse
}

type createAPIKeyReplayResponse struct {
	apiKeyResponse
	Message         string `json:"message"`
	SecretAvailable bool   `json:"secret_available"`
}

type apiKeyPage struct {
	Items      []apiKeyResponse `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

type adminTokenRevokeResponse struct {
	PublicID    string     `json:"public_id"`
	PrincipalID string     `json:"principal_id"`
	Status      string     `json:"status"`
	RevokedAt   *time.Time `json:"revoked_at"`
	AuthVersion uint64     `json:"auth_version"`
}
