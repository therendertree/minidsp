// Package money converts OpenRTB decimal prices (CPM, USD) to integer
// micro-USD and back, without ever using floating point.
//
// 1 USD = 1_000_000 micro-USD. A CPM is the price per 1000 impressions;
// dividing by 1000 to get a per-impression cost is a separate, explicit step
// done by whoever needs it (not here).
package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Micros is an amount in micro-USD.
type Micros int64

// PerUSD is the number of micro-USD in one USD.
const PerUSD Micros = 1_000_000

var (
	ErrSyntax   = errors.New("money: invalid decimal number")
	ErrNegative = errors.New("money: negative amount")
	ErrRange    = errors.New("money: amount out of range")
)

const (
	maxInputLen = 64  // longest accepted decimal text
	maxExponent = 100 // largest |exponent| accepted in 1e<N> notation
	maxDigits   = 18  // keeps results safely inside int64
)

// ParseCPM parses a JSON-style decimal number ("2.5", "0.0000001", "1e3")
// into micro-USD. Anything finer than one micro-USD is rounded UP, so a
// floor is never understated. Negative amounts are rejected; "-0" is zero.
func ParseCPM(s string) (Micros, error) {
	if len(s) == 0 {
		return 0, ErrSyntax
	}
	if len(s) > maxInputLen {
		return 0, ErrRange
	}
	i := 0
	neg := false
	if s[i] == '-' {
		neg = true
		i++
	}

	start := i
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	intPart := s[start:i]
	if intPart == "" {
		return 0, ErrSyntax
	}

	fracPart := ""
	if i < len(s) && s[i] == '.' {
		i++
		start = i
		for i < len(s) && isDigit(s[i]) {
			i++
		}
		fracPart = s[start:i]
		if fracPart == "" {
			return 0, ErrSyntax
		}
	}

	exp := 0
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		start = i
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		digitsStart := i
		for i < len(s) && isDigit(s[i]) {
			i++
		}
		if i == digitsStart {
			return 0, ErrSyntax
		}
		var err error
		exp, err = strconv.Atoi(s[start:i])
		if err != nil || exp > maxExponent || exp < -maxExponent {
			return 0, ErrRange
		}
	}
	if i != len(s) {
		return 0, ErrSyntax
	}

	// value = digits * 10^(exp - len(frac)); in micro-USD that is
	// digits * 10^e10 with e10 = exp - len(frac) + 6.
	digits := strings.TrimLeft(intPart+fracPart, "0")
	if digits == "" {
		return 0, nil // zero, including "-0"
	}
	if neg {
		return 0, ErrNegative
	}
	e10 := exp - len(fracPart) + 6

	if e10 >= 0 {
		if len(digits)+e10 > maxDigits {
			return 0, ErrRange
		}
		v, err := strconv.ParseInt(digits+strings.Repeat("0", e10), 10, 64)
		if err != nil {
			return 0, ErrRange
		}
		return Micros(v), nil
	}

	keep := len(digits) + e10 // digits left after dropping sub-micro digits
	if keep <= 0 {
		return 1, nil // positive but below one micro-USD: round up
	}
	head, tail := digits[:keep], digits[keep:]
	if len(head) > maxDigits {
		return 0, ErrRange
	}
	v, err := strconv.ParseInt(head, 10, 64)
	if err != nil {
		return 0, ErrRange
	}
	if strings.Trim(tail, "0") != "" {
		v++
	}
	return Micros(v), nil
}

// FormatCPM renders m as a JSON number in USD with the fewest decimals:
// 2_500_000 -> "2.5", 3_000_000 -> "3", 1 -> "0.000001".
func FormatCPM(m Micros) string {
	sign := ""
	if m < 0 {
		sign = "-"
		m = -m
	}
	whole, frac := m/PerUSD, m%PerUSD
	if frac == 0 {
		return fmt.Sprintf("%s%d", sign, whole)
	}
	f := strings.TrimRight(fmt.Sprintf("%06d", frac), "0")
	return fmt.Sprintf("%s%d.%s", sign, whole, f)
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
