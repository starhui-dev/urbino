// Package httpapi hosts the Urbino HTTP listeners.
//
// Stage 00 implements the internal health listener only. There is no public
// data plane, no admin plane and no model route: this package cannot serve any
// inference or management request.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// HealthPath is the only route of the internal health listener.
const HealthPath = "/healthz"

const (
	readHeaderTimeout = 5 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 30 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// healthBody is the fixed response of HealthPath.
var healthBody = []byte(`{"status":"ok"}` + "\n")

// NewHealthHandler returns the handler of the internal health listener. It
// serves exactly one route, GET /healthz, answering 200 with a JSON body. Any
// other path answers 404 and any other method on /healthz answers 405, so the
// listener cannot be mistaken for the public or admin surface.
func NewHealthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(HealthPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(healthBody)
	})
	return mux
}

// HealthServer is the internal health listener. It is created with
// NewHealthServer, bound with Listen (or implicitly by Serve) and stopped by
// cancelling the context passed to Serve.
type HealthServer struct {
	addr    string
	handler http.Handler
	server  *http.Server
	ln      net.Listener
}

// NewHealthServer prepares the internal health listener for addr, which must be
// a "host:port" address. Loopback is the documented default: binding every
// interface is an explicit operator decision made outside this constructor.
func NewHealthServer(addr string) (*HealthServer, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, errors.New("httpapi: health listener address is empty")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, fmt.Errorf("httpapi: invalid health listener address %q: %w", addr, err)
	}
	handler := NewHealthHandler()
	return &HealthServer{
		addr:    addr,
		handler: handler,
		server: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
	}, nil
}

// Handler returns the handler served by the health listener.
func (s *HealthServer) Handler() http.Handler {
	return s.handler
}

// Listen binds the health listener and reports the bound address through Addr.
func (s *HealthServer) Listen() error {
	if s.ln != nil {
		return errors.New("httpapi: health listener is already bound")
	}
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("httpapi: listen on %q: %w", s.addr, err)
	}
	s.ln = ln
	return nil
}

// Addr returns the bound listener address, or nil before Listen.
func (s *HealthServer) Addr() net.Addr {
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Close releases a bound listener that is not being served.
func (s *HealthServer) Close() error {
	if s.ln == nil {
		return nil
	}
	err := s.ln.Close()
	return err
}

// Serve binds the listener when needed and serves until ctx is done, then shuts
// the listener down gracefully and returns nil. A listener failure is returned
// as-is; a context that is already done before serving is reported as
// ctx.Err() so a caller can tell cancellation from a serving failure.
func (s *HealthServer) Serve(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.ln == nil {
		if err := s.Listen(); err != nil {
			return err
		}
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.Serve(s.ln)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("httpapi: health listener: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			_ = s.server.Close()
			return fmt.Errorf("httpapi: shutdown health listener: %w", err)
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("httpapi: health listener: %w", err)
		}
		return nil
	}
}
