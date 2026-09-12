package domain

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/starhui-dev/urbino/internal/clock"
)

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

func TestUUIDv7InjectableAndRFCVariant(t *testing.T) {
	seed := bytes.Repeat([]byte{0x11}, 16)
	u, err := NewUUIDv7(fixedClock{time.UnixMilli(1735689600123)}, bytes.NewReader(seed))
	if err != nil {
		t.Fatal(err)
	}
	if u.Version() != 7 || !u.VariantRFC4122() {
		t.Fatalf("bad UUIDv7 %s", u)
	}
	if !strings.HasPrefix(u.String(), "01941f29-7c7b-") {
		t.Fatalf("timestamp not encoded: %s", u)
	}
	if _, err := ParseUUID(u.String()); err != nil {
		t.Fatal(err)
	}
	_ = clock.Real{}
}

func TestMoneyBoundaries(t *testing.T) {
	m, err := ParseMoney("USD", "1.2")
	if err != nil || m.AmountMicros != 1_200_000 || m.Format() != "USD 1.200000" {
		t.Fatalf("money parse: %#v %v", m, err)
	}
	if _, err := ParseMoney("usd", "1"); err == nil {
		t.Fatal("lowercase currency accepted")
	}
	if _, err := ParseMoney("USD", "-1"); err == nil {
		t.Fatal("negative accepted")
	}
	if _, err := ParseMoney("USD", "9223372036855.808"); err == nil {
		t.Fatal("overflow accepted")
	}
	if _, err := m.Add(Money{Currency: "EUR", AmountMicros: 1}); err == nil {
		t.Fatal("currency mismatch accepted")
	}
	if _, err := m.Sub(Money{Currency: "USD", AmountMicros: m.AmountMicros + 1}); err == nil {
		t.Fatal("negative result accepted")
	}
	max, err := NewMoney("USD", int64(^uint64(0)>>1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := max.Add(m); err == nil {
		t.Fatal("addition overflow accepted")
	}
}

func TestRequestTerminalCannotReopen(t *testing.T) {
	if err := RequestTransition(RequestPending, RequestAdmitted); err != nil {
		t.Fatal(err)
	}
	if err := RequestTransition(RequestSucceeded, RequestStreaming); err == nil {
		t.Fatal("terminal state reopened")
	}
	if err := RequestTransition(RequestPending, RequestStatus("future")); err == nil {
		t.Fatal("unknown state accepted")
	}
}

func TestUnknownCapabilityAndScopeRejected(t *testing.T) {
	if _, err := ParseCapability("future.capability"); err == nil {
		t.Fatal("unknown capability accepted")
	}
	if _, err := ParseScope("tenant:admin"); err == nil {
		t.Fatal("unknown scope accepted")
	}
}

func TestUsageMissingIsNotZero(t *testing.T) {
	u := Usage{Completeness: UsagePartial, Source: UsageSourceUnknown, InputTotal: UnknownCount(), OutputTotal: UnknownCount()}
	if err := u.Validate(); err != nil {
		t.Fatal(err)
	}
	if u.InputTotal.Known {
		t.Fatal("unknown count became zero")
	}
	if _, err := KnownCount(-1); err == nil {
		t.Fatal("negative count accepted")
	}
	if err := (Usage{Completeness: UsagePartial, Source: UsageSource("future")}).Validate(); err == nil {
		t.Fatal("unknown usage source accepted")
	}
	if err := (Usage{Completeness: UsagePartial, Source: UsageSourceEstimate}).Validate(); err == nil {
		t.Fatal("estimate source without estimate flag accepted")
	}
}

func TestErrorDoesNotExposeCause(t *testing.T) {
	e := &Error{Code: ErrUnauthorized, Message: "unauthorized", Cause: &secretErr{}}
	if strings.Contains(e.Error(), "super-secret") {
		t.Fatal("secret leaked")
	}
}

type secretErr struct{}

func (*secretErr) Error() string { return "super-secret" }
