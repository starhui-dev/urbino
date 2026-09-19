package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Currency is an explicit three-letter settlement currency.
type Currency string

var supportedCurrencies = map[Currency]struct{}{
	"CNY": {},
	"EUR": {},
	"GBP": {},
	"JPY": {},
	"USD": {},
}

func ParseCurrency(value string) (Currency, error) {
	currency := Currency(strings.ToUpper(strings.TrimSpace(value)))
	if _, ok := supportedCurrencies[currency]; !ok {
		return "", fmt.Errorf("invalid currency %q", value)
	}
	return currency, nil
}

// Money stores non-negative micro-units. Currency is never implicit.
type Money struct {
	Currency Currency
	Micros   int64
}

func NewMoney(currency Currency, micros int64) (Money, error) {
	parsed, err := ParseCurrency(string(currency))
	if err != nil {
		return Money{}, err
	}
	if micros < 0 {
		return Money{}, fmt.Errorf("money cannot be negative")
	}
	return Money{Currency: parsed, Micros: micros}, nil
}

func ParseMoney(currency Currency, value string) (Money, error) {
	parsed, err := ParseCurrency(string(currency))
	if err != nil {
		return Money{}, err
	}
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		return Money{}, fmt.Errorf("money must be a non-negative decimal")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return Money{}, fmt.Errorf("invalid money %q", value)
	}
	if len(parts) == 2 && len(parts[1]) > 6 {
		return Money{}, fmt.Errorf("money precision exceeds 6 decimals")
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return Money{}, fmt.Errorf("invalid money %q", value)
			}
		}
	}
	whole, err := strconv.ParseUint(parts[0], 10, 63)
	if err != nil || whole > math.MaxInt64/1_000_000 {
		return Money{}, fmt.Errorf("money overflows micro-units")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	fraction += strings.Repeat("0", 6-len(fraction))
	micros := int64(whole)*1_000_000 + int64(parseDigits(fraction))
	if micros < 0 {
		return Money{}, fmt.Errorf("money overflows micro-units")
	}
	return Money{Currency: parsed, Micros: micros}, nil
}

func parseDigits(value string) uint64 {
	var result uint64
	for _, r := range value {
		result = result*10 + uint64(r-'0')
	}
	return result
}

func (m Money) Validate() error {
	if _, err := ParseCurrency(string(m.Currency)); err != nil {
		return err
	}
	if m.Micros < 0 {
		return fmt.Errorf("money cannot be negative")
	}
	return nil
}

func (m Money) Add(other Money) (Money, error) {
	if err := m.Validate(); err != nil {
		return Money{}, err
	}
	if err := other.Validate(); err != nil {
		return Money{}, err
	}
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("cannot add different currencies")
	}
	if other.Micros > math.MaxInt64-m.Micros {
		return Money{}, fmt.Errorf("money addition overflows micro-units")
	}
	return Money{Currency: m.Currency, Micros: m.Micros + other.Micros}, nil
}

func (m Money) Multiply(factor uint64) (Money, error) {
	if err := m.Validate(); err != nil {
		return Money{}, err
	}
	if factor != 0 && uint64(m.Micros) > uint64(math.MaxInt64)/factor {
		return Money{}, fmt.Errorf("money multiplication overflows micro-units")
	}
	return Money{Currency: m.Currency, Micros: int64(uint64(m.Micros) * factor)}, nil
}

func (m Money) String() string {
	whole := m.Micros / 1_000_000
	fraction := m.Micros % 1_000_000
	return fmt.Sprintf("%s %d.%06d", m.Currency, whole, fraction)
}
