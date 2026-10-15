package ui

import (
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// The reference formats every number with Python's "," grouping
// (f"{value:,}"), so a 2 200 MHz clock reads "2,200 MHz" and a 1 023-byte
// pool reads "1,023 B". Without it the same reading renders differently in
// the two implementations, which is exactly what the port must not do.
func TestFmtGroupsThousands(t *testing.T) {
	cases := []struct {
		in     float64
		digits int
		want   string
	}{
		{999, 0, "999"},
		{1000, 0, "1,000"},
		{2200, 0, "2,200"},
		{2104, 0, "2,104"},
		{1234567, 0, "1,234,567"},
		{-1234, 0, "-1,234"},
		{45.678, 1, "45.7"},
		{1234.5, 1, "1,234.5"},
	}
	for _, c := range cases {
		if got := Fmt(&c.in, "", c.digits); got != c.want {
			t.Errorf("Fmt(%v, %d) = %q, want %q", c.in, c.digits, got, c.want)
		}
	}
}

func TestFmtPercentGroupsThousands(t *testing.T) {
	v := 1234.5
	if got := FmtPercent(&v, 0); got != "1,234%" {
		t.Fatalf("FmtPercent(1234.5) = %q, want 1,234%%", got)
	}
	v = 12.34
	if got := FmtPercent(&v, 1); got != "12.3%" {
		t.Fatalf("FmtPercent(12.34, 1) = %q, want 12.3%%", got)
	}
}

func TestFmtBytesGroupsThousands(t *testing.T) {
	v := model.Bytes(1023)
	if got := FmtBytes(&v); got != "1,023 B" {
		t.Fatalf("FmtBytes(1023) = %q, want 1,023 B", got)
	}
	v = 1024
	if got := FmtBytes(&v); got != "1.0 KiB" {
		t.Fatalf("FmtBytes(1024) = %q, want 1.0 KiB", got)
	}
}
