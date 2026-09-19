package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/urbino/internal/auth"
)

func TestPublicMiddlewareRejectsAdminCredential(t *testing.T) {
	called := false
	handler := CredentialMiddleware{Mode: auth.PublicListener, AuthenticatePublic: func(context.Context, auth.Request) (auth.Principal, error) { return auth.Principal{}, nil }}.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodGet, "http://public.invalid/", nil)
	req.Header.Set("Authorization", "Bearer gw_admin_public.secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("public listener accepted admin credential: code=%d called=%v", response.Code, called)
	}
}

func TestAdminMiddlewareRejectsPublicCredential(t *testing.T) {
	called := false
	handler := CredentialMiddleware{Mode: auth.AdminListener, AuthenticateAdmin: func(context.Context, auth.Request) (auth.AdminPrincipal, error) { return auth.AdminPrincipal{}, nil }}.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodGet, "http://admin.invalid/", nil)
	req.Header.Set("Authorization", "Bearer gw_live_public.secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("admin listener accepted public credential: code=%d called=%v", response.Code, called)
	}
}

func TestPublicMiddlewareRejectsLookupFailureAndQueryKey(t *testing.T) {
	handler := CredentialMiddleware{Mode: auth.PublicListener, AuthenticatePublic: func(context.Context, auth.Request) (auth.Principal, error) {
		return auth.Principal{}, errors.New("not found")
	}}.WrapWithClock(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }), func() time.Time { return time.Unix(100, 0) })
	req := httptest.NewRequest(http.MethodGet, "http://public.invalid/?api_key=gw_live_secret", nil)
	req.Header.Set("Authorization", "Bearer gw_live_public.secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("query key status=%d", response.Code)
	}
}

func TestMiddlewareRejectsAuthenticationServiceFailure(t *testing.T) {
	called := false
	public := CredentialMiddleware{Mode: auth.PublicListener, AuthenticatePublic: func(context.Context, auth.Request) (auth.Principal, error) {
		return auth.Principal{}, errors.New("invalid secret")
	}}.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodGet, "http://public.invalid/", nil)
	req.Header.Set("Authorization", "Bearer gw_live_public.secret")
	response := httptest.NewRecorder()
	public.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("public auth failure reached handler: code=%d called=%v", response.Code, called)
	}

	called = false
	admin := CredentialMiddleware{Mode: auth.AdminListener, AuthenticateAdmin: func(context.Context, auth.Request) (auth.AdminPrincipal, error) {
		return auth.AdminPrincipal{}, errors.New("expired token")
	}}.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	req = httptest.NewRequest(http.MethodGet, "http://admin.invalid/", nil)
	req.Header.Set("Authorization", "Bearer gw_admin_public.secret")
	response = httptest.NewRecorder()
	admin.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("admin auth failure reached handler: code=%d called=%v", response.Code, called)
	}
}
