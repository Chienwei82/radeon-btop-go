package model

import "fmt"

// OdvUnit says what an overdrive entry is measured in.
type OdvUnit string

const (
	// OdvMillivolt is a legacy table: a bare integer in millivolts.
	OdvMillivolt OdvUnit = "mV"
	// OdvMegahertz is RDNA2 and later: a clock ceiling in megahertz.
	OdvMegahertz OdvUnit = "MHz"
)

// PowerProfileEntry is one selectable SCPP profile.
type PowerProfileEntry struct {
	Index  int
	Name   string
	Active bool
}

// PowerProfileTable is a parsed pp_power_profile_mode.
type PowerProfileTable struct {
	Profiles []PowerProfileEntry
	Present  bool
}

// Active returns the starred profile, or the first one when none is marked.
func (t PowerProfileTable) Active() *PowerProfileEntry {
	if len(t.Profiles) == 0 {
		return nil
	}
	for i := range t.Profiles {
		if t.Profiles[i].Active {
			return &t.Profiles[i]
		}
	}
	return &t.Profiles[0]
}

// ActiveName returns the active profile's name, or "" when absent.
func (t PowerProfileTable) ActiveName() string {
	if e := t.Active(); e != nil {
		return e.Name
	}
	return ""
}

// ActiveIndex returns the active profile's id, or nil when absent.
func (t PowerProfileTable) ActiveIndex() *int {
	if e := t.Active(); e != nil {
		v := e.Index
		return &v
	}
	return nil
}

// Names returns every profile name, in table order.
func (t PowerProfileTable) Names() []string {
	out := make([]string, 0, len(t.Profiles))
	for _, p := range t.Profiles {
		out = append(out, p.Name)
	}
	return out
}

// Describe returns a one-line summary, e.g. "3D_FULL_SCREEN (2 of 7)".
func (t PowerProfileTable) Describe() string {
	e := t.Active()
	if e == nil {
		return "N/A"
	}
	return fmt.Sprintf("%s (%d of %d)", e.Name, e.Index, len(t.Profiles))
}

// OdvEntry is one overdrive table entry: a value for one DPM state.
type OdvEntry struct {
	State int
	Value int
	Unit  OdvUnit
}

// OdvDomain is one clock domain's overdrive table.
type OdvDomain struct {
	Name    string
	Entries []OdvEntry
	Minimum *int
	Maximum *int
	Unit    OdvUnit
}

// domainLabels are display names for the overdrive domains.
var domainLabels = map[string]string{
	"sclk":   "Shader clock",
	"mclk":   "Memory clock",
	"socclk": "SoC clock",
}

// Label returns the domain's display name.
func (d OdvDomain) Label() string {
	if l, ok := domainLabels[d.Name]; ok {
		return l
	}
	return upperASCII(d.Name)
}

// Ceiling returns the highest value listed in the table.
func (d OdvDomain) Ceiling() *int {
	if len(d.Entries) == 0 {
		return nil
	}
	top := d.Entries[0].Value
	for _, e := range d.Entries[1:] {
		if e.Value > top {
			top = e.Value
		}
	}
	return &top
}

// Floor returns the lowest value listed in the table.
func (d OdvDomain) Floor() *int {
	if len(d.Entries) == 0 {
		return nil
	}
	low := d.Entries[0].Value
	for _, e := range d.Entries[1:] {
		if e.Value < low {
			low = e.Value
		}
	}
	return &low
}

// Headroom returns what is left between the ceiling and the domain maximum.
func (d OdvDomain) Headroom() *int {
	top, max := d.Ceiling(), d.Maximum
	if top == nil || max == nil {
		return nil
	}
	v := *max - *top
	return &v
}

// Percent returns where the ceiling sits between floor and maximum.
func (d OdvDomain) Percent() *Percent {
	if d.Minimum == nil || d.Maximum == nil {
		return nil
	}
	top := d.Ceiling()
	if top == nil || *d.Maximum <= *d.Minimum {
		return nil
	}
	v := clampPercent(float64(*top-*d.Minimum) / float64(*d.Maximum-*d.Minimum) * 100.0)
	return &v
}

// Describe returns a compact one-line summary such as "2104/2600 MHz".
func (d OdvDomain) Describe() string {
	unit := string(d.Unit)
	top := d.Ceiling()
	if top == nil {
		return fmtOdvRange(d.Minimum, d.Maximum, unit)
	}
	if d.Minimum == nil || d.Maximum == nil {
		return fmt.Sprintf("%d %s", *top, unit)
	}
	return fmt.Sprintf("%d/%d %s", *top, *d.Maximum, unit)
}

// OdcVoltage is a parsed pp_od_clk_voltage.
type OdcVoltage struct {
	Domains        []OdvDomain
	VddgfxOffsetMv *int
	Present        bool
}

// Domain looks up one domain by key, e.g. "sclk".
func (o OdcVoltage) Domain(name string) *OdvDomain {
	for i := range o.Domains {
		if o.Domains[i].Name == name {
			return &o.Domains[i]
		}
	}
	return nil
}

// Describe returns a one-line summary of every domain.
func (o OdcVoltage) Describe() string {
	if len(o.Domains) == 0 {
		return "N/A"
	}
	out := ""
	for i, d := range o.Domains {
		if i > 0 {
			out += " · "
		}
		out += d.Label() + " " + d.Describe()
	}
	return out
}

func fmtOdvRange(low, high *int, unit string) string {
	return fmtVal(low, unit) + " - " + fmtVal(high, unit)
}

func fmtVal(v *int, unit string) string {
	if v == nil {
		return "N/A"
	}
	return fmt.Sprintf("%d %s", *v, unit)
}

func upperASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}
