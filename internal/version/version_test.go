package version

import "testing"

func TestStringReportsInjectedVersion(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })

	Version = "1.2.3"
	if got := String(); got != "1.2.3" {
		t.Fatalf("String() = %q, want %q", got, "1.2.3")
	}

	Version = "  "
	if got := String(); got != Development {
		t.Fatalf("String() for blank version = %q, want %q", got, Development)
	}
}
