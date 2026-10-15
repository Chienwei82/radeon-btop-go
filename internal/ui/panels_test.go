package ui

import "testing"

// RED: panel builders do not exist yet.
func TestStatRowRendersLabelBarAndValue(t *testing.T) {
	v := 42.0
	row := Plain(StatRow("GPU", "42%", &v, GetTheme("default"), 40, nil))
	if n := len([]rune(row)); n == 0 || n > 40 {
		t.Fatalf("row runes = %d, want 1..40: %q", n, row)
	}
	if !contains(row, "GPU") || !contains(row, "42%") {
		t.Fatalf("row = %q, want label and value", row)
	}
}

func TestStatRowMissingShowsNANotZero(t *testing.T) {
	row := Plain(StatRow("GPU", NA, nil, GetTheme("default"), 40, nil))
	if !contains(row, NA) {
		t.Fatalf("row = %q, want N/A", row)
	}
	if contains(row, "0%") {
		t.Fatalf("row = %q, missing must not render as 0%%", row)
	}
	if contains(row, BarFilled) {
		t.Fatalf("row = %q, missing must draw an empty bar", row)
	}
}

func TestHeaderShowsIdentityAndTiming(t *testing.T) {
	m := testMetrics()
	text := Plain(HeaderPanel(m, GetTheme("default"), 3725.0, 1.0))
	for _, want := range []string{"AMD Radeon Test", "dGPU", "amdgpu", "0000:0c:00.0", "0x73bf", "renderD128"} {
		if !contains(text, want) {
			t.Fatalf("header = %q, want %q", text, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
