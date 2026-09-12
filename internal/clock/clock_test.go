package clock

import (
	"testing"
	"time"
)

type fixed struct{ value time.Time }

func (f fixed) Now() time.Time { return f.value }

func TestClockCanBeInjected(t *testing.T) {
	want := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if got := (fixed{value: want}).Now(); !got.Equal(want) {
		t.Fatalf("Now() = %v, want %v", got, want)
	}
}
