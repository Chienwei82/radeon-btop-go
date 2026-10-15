package ui

import (
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// RED: full theme + format parity with the Python reference.
func TestRequiredThemesExist(t *testing.T) {
	for _, name := range model.ThemeNames {
		if _, ok := Themes[name]; !ok {
			t.Fatalf("theme %q missing from ui.Themes", name)
		}
	}
}

func TestThemeLookupIsCaseInsensitive(t *testing.T) {
	if got := GetTheme("DRACULA"); got.Name != "dracula" {
		t.Fatalf("GetTheme(DRACULA) = %q, want dracula", got.Name)
	}
}

func TestThresholdColourEscalates(t *testing.T) {
	th := GetTheme("default")
	mk := func(v float64) *float64 { return &v }
	if got := ThresholdStyle(th, nil); got != th.Muted {
		t.Fatalf("nil = %q, want muted %q", got, th.Muted)
	}
	if got := ThresholdStyle(th, mk(10)); got != th.OK {
		t.Fatalf("10%% = %q, want ok %q", got, th.OK)
	}
	if got := ThresholdStyle(th, mk(75)); got != th.Warn {
		t.Fatalf("75%% = %q, want warn %q", got, th.Warn)
	}
	if got := ThresholdStyle(th, mk(95)); got != th.Alert {
		t.Fatalf("95%% = %q, want alert %q", got, th.Alert)
	}
}

func TestDraculaAndGruvboxMidpoints(t *testing.T) {
	if got := NewGradient(GetTheme("dracula"), true).At(50); got != "#f1fa8c" {
		t.Fatalf("dracula mid = %q, want #f1fa8c", got)
	}
	if got := NewGradient(GetTheme("gruvbox"), true).At(50); got != "#fabd2f" {
		t.Fatalf("gruvbox mid = %q, want #fabd2f", got)
	}
}

func TestGradientStopCounts(t *testing.T) {
	if got := NewGradient(GetTheme("default"), true).Len(); got != TrueColorStops {
		t.Fatalf("truecolor stops = %d, want %d", got, TrueColorStops)
	}
	if got := NewGradient(GetTheme("default"), false).Len(); got != ANSIStops {
		t.Fatalf("ansi stops = %d, want %d", got, ANSIStops)
	}
}

func TestGradientClampsOutOfRange(t *testing.T) {
	g := NewGradient(GetTheme("default"), true)
	if g.At(-10) != g.At(0) || g.At(250) != g.At(100) {
		t.Fatal("gradient must clamp out-of-range values to the ramp ends")
	}
}

func TestANSIGradientSnapsToCube(t *testing.T) {
	g := NewGradient(GetTheme("default"), false)
	for _, stop := range g.Stops() {
		rgb := hexToRGB(stop)
		for _, ch := range rgb {
			ok := false
			for _, level := range CubeLevels {
				if ch == level {
					ok = true
				}
			}
			if !ok {
				t.Fatalf("ansi stop %q channel %d not a cube level", stop, ch)
			}
		}
	}
}

func TestEveryColourIsValidHex(t *testing.T) {
	for name, th := range Themes {
		for _, c := range []string{th.Low, th.Mid, th.High, th.Accent, th.Text, th.Muted, th.Track, th.OK, th.Warn, th.Alert} {
			if len(c) != 7 || c[0] != '#' || strings.Trim(c[1:], "0123456789abcdefABCDEF") != "" {
				t.Fatalf("theme %q colour %q is not #rrggbb", name, c)
			}
		}
	}
}

func TestSnapToCubePicksNearestLevel(t *testing.T) {
	if got := snapToCube([3]int{100, 100, 100}); got != [3]int{95, 95, 95} {
		t.Fatalf("snap(100) = %v, want 95s", got)
	}
	if got := snapToCube([3]int{200, 200, 200}); got != [3]int{215, 215, 215} {
		t.Fatalf("snap(200) = %v, want 215s", got)
	}
}

// --- format ---

func TestFmtMissingIsNANotZero(t *testing.T) {
	if got := Fmt(nil, "", 0); got != NA {
		t.Fatalf("Fmt(nil) = %q, want N/A", got)
	}
	if got := FmtTemp(nil); got != NA {
		t.Fatalf("FmtTemp(nil) = %q, want N/A", got)
	}
	if got := FmtPercent(nil, 0); got != NA {
		t.Fatalf("FmtPercent(nil) = %q, want N/A", got)
	}
	if got := FmtBytes(nil); got != NA {
		t.Fatalf("FmtBytes(nil) = %q, want N/A", got)
	}
}

func TestFmtBytesUnits(t *testing.T) {
	cases := map[int64]string{
		512:                    "512 B",
		2048:                   "2.0 KiB",
		3 * 1024 * 1024:        "3.0 MiB",
		8 * 1024 * 1024 * 1024: "8.0 GiB",
	}
	for in, want := range cases {
		v := in
		if got := FmtBytes(&v); got != want {
			t.Fatalf("FmtBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestBarMissingIsNA(t *testing.T) {
	if got := Bar(nil, 10); got != NA {
		t.Fatalf("Bar(nil) = %q, want N/A", got)
	}
}

func TestBarExactWidth(t *testing.T) {
	v := 50.0
	got := Bar(&v, 10)
	if len([]rune(got)) != 10 {
		t.Fatalf("Bar(50%%, 10) = %q (len %d), want width 10", got, len([]rune(got)))
	}
	if want := "█████░░░░░"; got != want {
		t.Fatalf("Bar(50%%, 10) = %q, want %q", got, want)
	}
}

func TestFmtDuration(t *testing.T) {
	cases := map[float64]string{
		9:     "9s",
		65:    "1m 05s",
		3700:  "1h 01m",
		90000: "1d 01h",
	}
	for in, want := range cases {
		v := in
		if got := FmtDuration(&v); got != want {
			t.Fatalf("FmtDuration(%v) = %q, want %q", in, got, want)
		}
	}
}
