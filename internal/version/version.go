// Package version exposes the Urbino build version.
//
// Version is a build-time variable so release builds can inject the real
// program version:
//
//	go build -ldflags "-X example.com/urbino/internal/version.Version=1.2.3" ./cmd/urbino
//
// A build that injects nothing reports Development. The value is the version of
// this binary, never the version of a Go module or dependency, and it must not
// carry credentials, tokens or other secrets.
package version

import "strings"

// Development is the version reported by builds that inject no version.
const Development = "dev"

// Version is the version of this binary, injected at build time.
var Version = Development

// String returns the effective version, falling back to Development when the
// injected value is blank.
func String() string {
	if v := strings.TrimSpace(Version); v != "" {
		return v
	}
	return Development
}
