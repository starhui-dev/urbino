package httpapi

import (
	"context"
	"net/http"
	"time"

	"example.com/urbino/internal/auth"
)

// CredentialMiddleware enforces listener-specific authentication. It never
// trusts forwarded addresses; the caller may pass RemoteAddr as the limiter
// identity until a separately reviewed proxy policy exists.
type CredentialMiddleware struct {
	Mode               auth.ListenerMode
	Limiter            *auth.FailureLimiter
	AuthenticatePublic func(context.Context, auth.Request) (auth.Principal, error)
	AuthenticateAdmin  func(context.Context, auth.Request) (auth.AdminPrincipal, error)
}
type publicPrincipalContextKey struct{}
type adminPrincipalContextKey struct{}

func PublicPrincipal(ctx context.Context) (auth.Principal, bool) {
	principal, ok := ctx.Value(publicPrincipalContextKey{}).(auth.Principal)
	return principal, ok
}

func AdminPrincipal(ctx context.Context) (auth.AdminPrincipal, bool) {
	principal, ok := ctx.Value(adminPrincipalContextKey{}).(auth.AdminPrincipal)
	return principal, ok
}

func (m CredentialMiddleware) Wrap(next http.Handler) http.Handler {
	return m.WrapWithClock(next, time.Now)
}

// WrapWithClock is the testable entrypoint; the clock is explicit so tests do
// not rely on wall-clock sleeps.
func (m CredentialMiddleware) WrapWithClock(next http.Handler, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fail := func() {
			if m.Limiter != nil {
				m.Limiter.RecordFailure(r.RemoteAddr, now())
			}
			m.reject(w)
		}
		if m.Limiter != nil && !m.Limiter.Check(r.RemoteAddr, now()) {
			m.reject(w)
			return
		}
		if _, err := auth.ParseHeaders(r.Header, r.URL.Query(), m.Mode); err != nil {
			fail()
			return
		}
		request := auth.Request{Method: r.Method, Path: r.URL.Path, Header: r.Header, Query: r.URL.Query(), RemoteAddr: r.RemoteAddr}
		if m.Mode == auth.AdminListener {
			if m.AuthenticateAdmin == nil {
				fail()
				return
			}
			principal, err := m.AuthenticateAdmin(r.Context(), request)
			if err != nil {
				fail()
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), adminPrincipalContextKey{}, principal))
			next.ServeHTTP(w, r)
			return
		}
		if m.Mode != auth.PublicListener || m.AuthenticatePublic == nil {
			fail()
			return
		}
		principal, err := m.AuthenticatePublic(r.Context(), request)
		if err != nil {
			fail()
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), publicPrincipalContextKey{}, principal))
		next.ServeHTTP(w, r)
	})
}

func (m CredentialMiddleware) reject(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"code":"unauthorized","message":"authentication failed"}` + "\n"))
}
