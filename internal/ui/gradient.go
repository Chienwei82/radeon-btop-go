package ui

// TrueColorStops and ANSIStops are the gradient stop counts. Both are
// deliberately odd so that a stop lands exactly on the 0.5 position,
// which makes 50% render as the theme's mid colour rather than a hair
// either side of it.
const (
	TrueColorStops = 255
	ANSIStops      = 25
)

// CubeLevels are the channel levels of the xterm 256-colour 6x6x6 cube.
// Index * 40 + 55 (indexes 1-5; level 0 maps to palette slot 16) is the
// palette slot.
var CubeLevels = [6]int{0, 95, 135, 175, 215, 255}

// Gradient is a precomputed colour ramp for one theme and one colour depth.
//
// The ramp is built once and indexed by value, so drawing a graph performs
// no interpolation at paint time.
type Gradient struct {
	stops     []string
	truecolor bool
}

// NewGradient builds the ramp for a theme. When truecolor is false every
// stop is snapped to the 256-colour cube first.
func NewGradient(th Theme, truecolor bool) Gradient {
	count := TrueColorStops
	if !truecolor {
		count = ANSIStops
	}
	low := hexToRGB(th.Low)
	mid := hexToRGB(th.Mid)
	high := hexToRGB(th.High)
	stops := make([]string, 0, count)
	for i := range count {
		position := float64(i) / float64(count-1)
		var rgb [3]int
		if position <= 0.5 {
			rgb = mix(low, mid, position*2)
		} else {
			rgb = mix(mid, high, (position-0.5)*2)
		}
		if !truecolor {
			rgb = snapToCube(rgb)
		}
		stops = append(stops, rgbToHex(rgb))
	}
	return Gradient{stops: stops, truecolor: truecolor}
}

// Len returns the number of stops in the ramp.
func (g Gradient) Len() int { return len(g.stops) }

// At returns the colour for a 0-100 value.
//
// The value is clamped rather than rejected: a slightly out-of-range
// reading is ordinary on a busy machine, and the top of the ramp is the
// correct answer.
func (g Gradient) At(percent float64) string {
	clamped := min(100.0, max(0.0, percent))
	index := int(clamped/100.0*float64(len(g.stops)-1) + 0.5)
	return g.stops[index]
}

// Stops exposes the ramp, for tests and for the theme preview.
func (g Gradient) Stops() []string { return append([]string(nil), g.stops...) }

// hexToRGB parses #rrggbb into a channel triple. A malformed colour
// degrades to white rather than exploding mid-render.
func hexToRGB(value string) [3]int {
	text := value
	if len(text) == 7 && text[0] == '#' {
		text = text[1:]
	}
	if len(text) != 6 {
		return [3]int{255, 255, 255}
	}
	var rgb [3]int
	for i := range 3 {
		v := hexByte(text[2*i], text[2*i+1])
		if v < 0 {
			return [3]int{255, 255, 255}
		}
		rgb[i] = v
	}
	return rgb
}

func hexByte(hi, lo byte) int {
	return hexNibble(hi)*16 + hexNibble(lo)
}

func hexNibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -16 // poison: hexByte of anything with this in it is negative
}

func rgbToHex(rgb [3]int) string {
	const digits = "0123456789abcdef"
	var b [7]byte
	b[0] = '#'
	for i, v := range rgb {
		b[1+2*i] = digits[(v>>4)&0xf]
		b[2+2*i] = digits[v&0xf]
	}
	return string(b[:])
}

// snapToCube moves each channel to the nearest cube level. This is what
// keeps a gradient monotonic once a 24-bit value has to survive a trip
// through a 256-colour terminal.
func snapToCube(rgb [3]int) [3]int {
	var out [3]int
	for i, channel := range rgb {
		best := CubeLevels[0]
		for _, level := range CubeLevels[1:] {
			if abs(level-channel) < abs(best-channel) {
				best = level
			}
		}
		out[i] = best
	}
	return out
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// mix blends two colours; t of 0 yields start and 1 yields end.
func mix(start, end [3]int, t float64) [3]int {
	return [3]int{
		int(lerp(float64(start[0]), float64(end[0]), t) + 0.5),
		int(lerp(float64(start[1]), float64(end[1]), t) + 0.5),
		int(lerp(float64(start[2]), float64(end[2]), t) + 0.5),
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && isSpace(s[start]) {
		start++
	}
	end := len(s)
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
