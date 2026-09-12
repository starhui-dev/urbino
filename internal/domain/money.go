package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Money stores integer millionths of an explicitly declared ISO-like currency.
type Money struct {
	Currency     string
	AmountMicros int64
}

func validateCurrency(currency string) error {
	if len(currency) != 3 {
		return fmt.Errorf("invalid currency")
	}
	for _, c := range currency {
		if c < 'A' || c > 'Z' {
			return fmt.Errorf("invalid currency")
		}
	}
	return nil
}

func NewMoney(currency string, micros int64) (Money, error) {
	if err := validateCurrency(currency); err != nil {
		return Money{}, err
	}
	if micros < 0 {
		return Money{}, fmt.Errorf("negative money")
	}
	return Money{Currency: currency, AmountMicros: micros}, nil
}

// ParseMoney parses a non-negative decimal with at most six fractional digits.
func ParseMoney(currency, value string) (Money, error) {
	if err := validateCurrency(currency); err != nil {
		return Money{}, err
	}
	v := strings.TrimSpace(value)
	if v == "" || strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") {
		return Money{}, fmt.Errorf("invalid money")
	}
	parts := strings.Split(v, ".")
	if len(parts) > 2 || parts[0] == "" {
		return Money{}, fmt.Errorf("invalid money")
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
		if frac == "" {
			return Money{}, fmt.Errorf("invalid money")
		}
	}
	if len(frac) > 6 {
		return Money{}, fmt.Errorf("too many fractional digits")
	}
	whole, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil || whole > uint64(math.MaxInt64)/1_000_000 {
		return Money{}, fmt.Errorf("money overflow")
	}
	for len(frac) < 6 {
		frac += "0"
	}
	var f uint64
	if frac != "" {
		f, err = strconv.ParseUint(frac, 10, 64)
		if err != nil {
			return Money{}, fmt.Errorf("invalid money")
		}
	}
	micros := whole*1_000_000 + f
	if micros > math.MaxInt64 {
		return Money{}, fmt.Errorf("money overflow")
	}
	return Money{Currency: currency, AmountMicros: int64(micros)}, nil
}

func (m Money) Format() string {
	return fmt.Sprintf("%s %d.%06d", m.Currency, m.AmountMicros/1_000_000, m.AmountMicros%1_000_000)
}

func (m Money) Add(other Money) (Money, error) {
	if err := m.valid(); err != nil {
		return Money{}, err
	}
	if err := other.valid(); err != nil {
		return Money{}, err
	}
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("currency mismatch")
	}
	if other.AmountMicros > math.MaxInt64-m.AmountMicros {
		return Money{}, fmt.Errorf("money overflow")
	}
	return Money{Currency: m.Currency, AmountMicros: m.AmountMicros + other.AmountMicros}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if err := m.valid(); err != nil {
		return Money{}, err
	}
	if err := other.valid(); err != nil {
		return Money{}, err
	}
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("currency mismatch")
	}
	if other.AmountMicros > m.AmountMicros {
		return Money{}, fmt.Errorf("negative money")
	}
	return Money{Currency: m.Currency, AmountMicros: m.AmountMicros - other.AmountMicros}, nil
}

func (m Money) valid() error {
	if err := validateCurrency(m.Currency); err != nil {
		return err
	}
	if m.AmountMicros < 0 {
		return fmt.Errorf("negative money")
	}
	return nil
}
