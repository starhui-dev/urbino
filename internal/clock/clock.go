// Package clock defines the time source used by stateful code.
package clock

import "time"

// Clock supplies the current time. Production code uses Real; tests can inject
// a deterministic implementation without changing global time.
type Clock interface {
	Now() time.Time
}

// Real reads the process wall clock.
type Real struct{}

// Now implements Clock.
func (Real) Now() time.Time { return time.Now() }
