package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"example.com/urbino/internal/auth"
)

const (
	PublicIdentityPath = "/api/v1/me"
	AdminIdentityPath  = "/admin/v1/session"
)

type AuthenticatedServers struct {
	publicAddr string
	adminAddr  string
	public     *http.Server
	admin      *http.Server
	publicLn   net.Listener
	adminLn    net.Listener
}

func NewAuthenticatedServers(publicAddr, adminAddr string,
	publicAuth func(context.Context, auth.Request) (auth.Principal, error),
	adminAuth func(context.Context, auth.Request) (auth.AdminPrincipal, error),
) (*AuthenticatedServers, error) {
	publicAddr = strings.TrimSpace(publicAddr)
	adminAddr = strings.TrimSpace(adminAddr)
	if _, _, err := net.SplitHostPort(publicAddr); err != nil {
		return nil, fmt.Errorf("httpapi: invalid public listener address")
	}
	if _, _, err := net.SplitHostPort(adminAddr); err != nil {
		return nil, fmt.Errorf("httpapi: invalid admin listener address")
	}
	adminHost, _, _ := net.SplitHostPort(adminAddr)
	if !isLoopbackHost(adminHost) {
		return nil, errors.New("httpapi: admin listener must bind a loopback address")
	}
	limiter, err := auth.NewFailureLimiter(10, time.Minute)
	if err != nil {
		return nil, err
	}
	publicMux := http.NewServeMux()
	publicMux.Handle(PublicIdentityPath, CredentialMiddleware{
		Mode:               auth.PublicListener,
		Limiter:            limiter,
		AuthenticatePublic: publicAuth,
	}.Wrap(http.HandlerFunc(publicIdentityHandler)))
	adminMux := http.NewServeMux()
	adminMux.Handle(AdminIdentityPath, CredentialMiddleware{
		Mode:              auth.AdminListener,
		Limiter:           limiter,
		AuthenticateAdmin: adminAuth,
	}.Wrap(http.HandlerFunc(adminIdentityHandler)))
	return &AuthenticatedServers{
		publicAddr: publicAddr,
		adminAddr:  adminAddr,
		public:     &http.Server{Handler: publicMux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second},
		admin:      &http.Server{Handler: adminMux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second},
	}, nil
}
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func publicIdentityHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	principal, ok := PublicPrincipal(r.Context())
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id":  principal.TenantID.String(),
		"project_id": principal.ProjectID.String(),
		"user_id":    principal.UserID.String(),
		"key_id":     principal.KeyID,
		"scopes":     principal.Scopes,
		"models":     principal.Models,
	})
}

func adminIdentityHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	principal, ok := AdminPrincipal(r.Context())
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"admin_id": principal.AdminID.String(),
		"scopes":   principal.Scopes,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *AuthenticatedServers) PublicAddr() net.Addr {
	if s == nil || s.publicLn == nil {
		return nil
	}
	return s.publicLn.Addr()
}

func (s *AuthenticatedServers) AdminAddr() net.Addr {
	if s == nil || s.adminLn == nil {
		return nil
	}
	return s.adminLn.Addr()
}

func (s *AuthenticatedServers) Listen() error {
	if s.publicLn != nil || s.adminLn != nil {
		return errors.New("httpapi: authenticated listeners are already bound")
	}
	publicLn, err := net.Listen("tcp", s.publicAddr)
	if err != nil {
		return fmt.Errorf("httpapi: listen on public address: %w", err)
	}
	adminLn, err := net.Listen("tcp", s.adminAddr)
	if err != nil {
		_ = publicLn.Close()
		return fmt.Errorf("httpapi: listen on admin address: %w", err)
	}
	s.publicLn, s.adminLn = publicLn, adminLn
	return nil
}

func (s *AuthenticatedServers) Close() error {
	var first error
	if s.publicLn != nil {
		if err := s.publicLn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			first = err
		}
	}
	if s.adminLn != nil {
		if err := s.adminLn.Close(); err != nil && !errors.Is(err, net.ErrClosed) && first == nil {
			first = err
		}
	}
	return first
}

func (s *AuthenticatedServers) Serve(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.publicLn == nil {
		if err := s.Listen(); err != nil {
			return err
		}
	}
	errCh := make(chan error, 2)
	go func() { errCh <- s.public.Serve(s.publicLn) }()
	go func() { errCh <- s.admin.Serve(s.adminLn) }()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = s.shutdown()
			return err
		}
	case <-ctx.Done():
		if err := s.shutdown(); err != nil {
			return err
		}
	}
	return nil
}

func (s *AuthenticatedServers) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.public.Shutdown(ctx); err != nil {
		_ = s.public.Close()
		_ = s.admin.Close()
		return fmt.Errorf("httpapi: shutdown public listener: %w", err)
	}
	if err := s.admin.Shutdown(ctx); err != nil {
		_ = s.admin.Close()
		return fmt.Errorf("httpapi: shutdown admin listener: %w", err)
	}
	return nil
}
