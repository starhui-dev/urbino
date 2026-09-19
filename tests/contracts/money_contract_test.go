package contracts_test

// P01-T01 (checklists/test-matrix.csv: 金额边界): negative amounts, parse and
// arithmetic overflow, and undeclared currencies must be rejected instead of
// being silently clamped, truncated or wrapped. Money is an exact non-negative
// integer amount of micro-units (1 unit = 10^-6 of the declared currency).

import (
	"math"
	"testing"

	"example.com/urbino/internal/domain"
)

func mustMoney(t *testing.T, currency domain.Currency, value string) domain.Money {
	t.Helper()
	m, err := domain.ParseMoney(currency, value)
	if err != nil {
		t.Fatalf("ParseMoney(%q, %q): unexpected error: %v", currency, value, err)
	}
	return m
}

func TestP01T01MoneyParseBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		currency  domain.Currency
		value     string
		wantMicro int64
		wantErr   bool
	}{
		{name: "zero", currency: "USD", value: "0", wantMicro: 0},
		{name: "whole units", currency: "USD", value: "12", wantMicro: 12_000_000},
		{name: "fractional units", currency: "USD", value: "1.5", wantMicro: 1_500_000},
		{name: "single micro", currency: "USD", value: "0.000001", wantMicro: 1},
		{name: "exactly max int64 micros", currency: "USD", value: "9223372036854.775807", wantMicro: math.MaxInt64},

		{name: "negative business amount", currency: "USD", value: "-0.01", wantErr: true},
		{name: "explicit plus sign", currency: "USD", value: "+1", wantErr: true},
		{name: "empty input", currency: "USD", value: "", wantErr: true},
		{name: "non numeric", currency: "USD", value: "abc", wantErr: true},
		{name: "two decimal points", currency: "USD", value: "1.2.3", wantErr: true},
		{name: "missing whole part", currency: "USD", value: ".5", wantErr: true},
		{name: "sub micro precision must not be truncated", currency: "USD", value: "1.0000005", wantErr: true},
		{name: "whole part beyond micro scale", currency: "USD", value: "9223372036855.0", wantErr: true},
		{name: "fraction overflow wraps int64", currency: "USD", value: "9223372036854.999999", wantErr: true},
		{name: "magnitude beyond uint64", currency: "USD", value: "99999999999999999999", wantErr: true},
		{name: "undeclared currency", currency: "ZZZ", value: "1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseMoney(tt.currency, tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseMoney(%q, %q) = %+v, want error", tt.currency, tt.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMoney(%q, %q) unexpected error: %v", tt.currency, tt.value, err)
			}
			if got.Micros != tt.wantMicro {
				t.Fatalf("ParseMoney(%q, %q).Micros = %d, want %d", tt.currency, tt.value, got.Micros, tt.wantMicro)
			}
			if got.Currency != tt.currency {
				t.Fatalf("ParseMoney(%q, %q).Currency = %q, want %q", tt.currency, tt.value, got.Currency, tt.currency)
			}
		})
	}
}

func TestP01T01MoneyRejectsNegativeAmounts(t *testing.T) {
	if _, err := domain.NewMoney("USD", -1); err == nil {
		t.Fatal("NewMoney must reject negative micro-units")
	}
	if _, err := domain.ParseMoney("USD", "-1"); err == nil {
		t.Fatal("ParseMoney must reject negative amounts")
	}
	m, err := domain.NewMoney("USD", 0)
	if err != nil || m.Micros != 0 {
		t.Fatalf("zero amount must be valid, got %+v, err %v", m, err)
	}
}

func TestP01T01MoneyCurrencyDeclarations(t *testing.T) {
	if got, err := domain.ParseCurrency("USD"); err != nil || got != "USD" {
		t.Fatalf("ParseCurrency(\"USD\") = %q, %v; want USD, nil", got, err)
	}
	// Case normalisation is observable behaviour: a declared code is accepted
	// in any case and always yields the canonical upper-case code.
	if got, err := domain.ParseCurrency("usd"); err != nil || got != "USD" {
		t.Fatalf("ParseCurrency(\"usd\") = %q, %v; want canonical USD", got, err)
	}
	for _, undeclared := range []string{"", "ZZZ", "USDD", "US D", "us d"} {
		if got, err := domain.ParseCurrency(undeclared); err == nil {
			t.Errorf("ParseCurrency(%q) = %q, want error for undeclared currency", undeclared, got)
		}
	}
}

func TestP01T01MoneyArithmeticOverflow(t *testing.T) {
	max := mustMoney(t, "USD", "9223372036854.775807") // math.MaxInt64 micro-units
	one := mustMoney(t, "USD", "0.000001")
	zero := domain.Money{Currency: "USD", Micros: 0}

	if got, err := max.Add(zero); err != nil || got.Micros != math.MaxInt64 {
		t.Fatalf("max + 0 must stay exactly at the int64 boundary, got %+v, err %v", got, err)
	}
	if _, err := max.Add(one); err == nil {
		t.Fatal("addition overflow must be rejected, not wrapped")
	}
	eur := mustMoney(t, "EUR", "1")
	if _, err := one.Add(eur); err == nil {
		t.Fatal("adding different currencies must be rejected")
	}

	amount := mustMoney(t, "USD", "1.5")
	if got, err := amount.Multiply(3); err != nil || got.Micros != 4_500_000 {
		t.Fatalf("1.5 * 3 = %+v, %v; want 4.5 USD", got, err)
	}
	if got, err := amount.Multiply(0); err != nil || got.Micros != 0 {
		t.Fatalf("1.5 * 0 = %+v, %v; want 0", got, err)
	}
	if _, err := max.Multiply(2); err == nil {
		t.Fatal("multiplication overflow must be rejected, not wrapped")
	}
}
