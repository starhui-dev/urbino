//go:build !identityimpl

package identity

import (
	"context"
)

// New returns ErrUnbound in the default build: tests/identity is not linked
// to internal/auth until the `identityimpl` build tag selects
// binding_identityimpl.go. Tests treat ErrUnbound as an explicit skip; a skip
// is never a pass (AGENTS.md invariant 8).
func New(ctx context.Context, opts Options) (System, error) {
	return nil, ErrUnbound
}

// NewScopedStore returns ErrUnbound in the default build. The PostgreSQL
// binding is the main agent's integration responsibility.
func NewScopedStore(ctx context.Context, dsn string) (ScopedStore, error) {
	return nil, ErrUnbound
}
