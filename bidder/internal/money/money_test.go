package money

import (
	"errors"
	"strings"
	"testing"
)

func TestParseCPM(t *testing.T) {
	tests := []struct {
		in   string
		want Micros
		err  error
	}{
		{"0", 0, nil},
		{"-0", 0, nil},
		{"0.5", 500_000, nil},
		{"2.5", 2_500_000, nil},
		{"2.50", 2_500_000, nil},
		{"3", 3_000_000, nil},
		{"0.000001", 1, nil},
		{"0.0000001", 1, nil},         // sub-micro rounds up
		{"0.0000000000001", 1, nil},   // far below a micro still rounds up
		{"1.0000001", 1_000_001, nil}, // rounds up, not to nearest
		{"1.0000000", 1_000_000, nil}, // trailing zeros are not a remainder
		{"1.2345678", 1_234_568, nil},
		{"1e3", 1_000_000_000, nil},
		{"1E3", 1_000_000_000, nil},
		{"2.5e-1", 250_000, nil},
		{"25e-7", 3, nil}, // 0.0000025 USD = 2.5 micro -> 3
		{"0e50", 0, nil},
		{"-0.1", 0, ErrNegative},
		{"-1e-9", 0, ErrNegative},
		{"", 0, ErrSyntax},
		{"abc", 0, ErrSyntax},
		{"1.", 0, ErrSyntax},
		{".5", 0, ErrSyntax},
		{"1e", 0, ErrSyntax},
		{"1x", 0, ErrSyntax},
		{"1e999", 0, ErrRange},
		{"9999999999999", 0, ErrRange}, // 13 digits + 6 > 18
		{strings.Repeat("1", 65), 0, ErrRange},
	}
	for _, tc := range tests {
		got, err := ParseCPM(tc.in)
		if !errors.Is(err, tc.err) {
			t.Errorf("ParseCPM(%q) err = %v, want %v", tc.in, err, tc.err)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("ParseCPM(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestFormatCPM(t *testing.T) {
	tests := map[Micros]string{
		0:          "0",
		1:          "0.000001",
		500_000:    "0.5",
		2_500_000:  "2.5",
		3_000_000:  "3",
		1_234_568:  "1.234568",
		10_000_001: "10.000001",
	}
	for in, want := range tests {
		if got := FormatCPM(in); got != want {
			t.Errorf("FormatCPM(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	for _, m := range []Micros{0, 1, 999_999, 1_000_000, 2_500_000, 123_456_789} {
		got, err := ParseCPM(FormatCPM(m))
		if err != nil || got != m {
			t.Errorf("round trip %d -> %q -> %d (err %v)", m, FormatCPM(m), got, err)
		}
	}
}
