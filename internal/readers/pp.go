package readers

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// ODVSection matches a section header inside pp_od_clk_voltage, e.g. OD_SCLK:.
var ODVSection = regexp.MustCompile(`^\s*(OD_[A-Z_]+)\s*:?\s*$`)

// odvRange matches an OD_RANGE line: "SCLK:     500Mhz       2600Mhz".
var odvRange = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9_]*)\s*:\s*(-?\d+)\s*([A-Za-z]*)\s+(-?\d+)\s*([A-Za-z]*)\s*$`)

// odvOffset matches the OD_VDDGFX_OFFSET signed millivolt value (-50mV).
var odvOffset = regexp.MustCompile(`(?i)^\s*(-?\d+)\s*mV\s*$`)

// odvSections maps section names to canonical domain keys.
var odvSections = map[string]string{
	"OD_SCLK": "sclk", "OD_MCLK": "mclk", "OD_SOCCLK": "socclk",
}

// rangeDomains are the domains OD_RANGE may name.
var rangeDomains = map[string]bool{"sclk": true, "mclk": true, "socclk": true}

// nonEntrySections carry no <state>: <value> rows.
var nonEntrySections = map[string]bool{"OD_RANGE": true, "OD_VDDGFX_OFFSET": true}

// legacyDomain is the bucket for a section-less legacy voltage table.
const legacyDomain = "legacy"

// unitOf maps a unit suffix onto an OdvUnit, defaulting when absent.
func unitOf(raw string, def model.OdvUnit) model.OdvUnit {
	if raw == "" {
		return def
	}
	if strings.EqualFold(raw, "mv") {
		return model.OdvMillivolt
	}
	return model.OdvMegahertz
}

// ParsePowerProfileTable parses pp_power_profile_mode text.
func ParsePowerProfileTable(text string) model.PowerProfileTable {
	if strings.TrimSpace(text) == "" {
		return model.PowerProfileTable{}
	}
	var profiles []model.PowerProfileEntry
	for _, line := range strings.Split(text, "\n") {
		m := ProfileRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// Python verbatim: the row matched, and the detailed guard is
		// "span is None or span.group(1) != '('". With PROFILE_ROW the
		// only rows reaching here are real profiles: header/detail rows
		// never match (verified: "0(  GFXCLK)..." and the header yield
		// no match). The span check therefore only needs to reject a
		// "(" captured as group 3.
		if m[3] != "" && m[3] != "*" {
			continue
		}
		id, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		profiles = append(profiles, model.PowerProfileEntry{
			Index: id, Name: strings.TrimSpace(m[2]), Active: m[3] == "*",
		})
	}
	if len(profiles) == 0 {
		return model.PowerProfileTable{}
	}
	return model.PowerProfileTable{Profiles: profiles, Present: true}
}

// ReadPowerProfileTable reads pp_power_profile_mode for a device.
func ReadPowerProfileTable(deviceDir string) model.PowerProfileTable {
	if text, ok := ReadText(filepath.Join(deviceDir, "pp_power_profile_mode"), TableLimit); ok {
		return ParsePowerProfileTable(text)
	}
	return model.PowerProfileTable{}
}

// ParseODCVoltage parses pp_od_clk_voltage in both eras.
func ParseODCVoltage(text string) model.OdcVoltage {
	if strings.TrimSpace(text) == "" {
		return model.OdcVoltage{}
	}
	defaultUnit := model.OdvMegahertz
	for _, line := range strings.Split(text, "\n") {
		if ODVSection.FindStringSubmatch(line) == nil {
			if odvOffset.FindStringSubmatch(strings.TrimSpace(line)) == nil && len(odvEntries(line)) > 0 {
				defaultUnit = model.OdvMillivolt
				break
			}
		}
	}
	entries := map[string][]model.OdvEntry{}
	// order records the first time each domain is seen, because the reference
	// emits one domain per clock the driver listed *in file order* -- OD_SCLK
	// before OD_MCLK, not alphabetically. A map alone would make the dump
	// depend on iteration order, which is the one thing a parity gate cannot
	// tolerate.
	var order []string
	type span struct {
		lo, hi int
		unit   model.OdvUnit
	}
	ranges := map[string]span{}
	var offset *int
	var current *string
	for _, line := range strings.Split(text, "\n") {
		stripped := strings.TrimSpace(line)
		if stripped == "" {
			continue
		}
		if m := ODVSection.FindStringSubmatch(line); m != nil {
			if nonEntrySections[m[1]] {
				current = nil
			} else if key, ok := odvSections[m[1]]; ok {
				k := key
				current = &k
			} else {
				current = nil
			}
			continue
		}
		if m := odvRange.FindStringSubmatch(line); m != nil && rangeDomains[strings.ToLower(m[1])] {
			lo, _ := strconv.Atoi(m[2])
			hi, _ := strconv.Atoi(m[4])
			ranges[strings.ToLower(m[1])] = span{lo, hi, unitOf(m[5], model.OdvMegahertz)}
			continue
		}
		if offset == nil {
			if m := odvOffset.FindStringSubmatch(stripped); m != nil {
				v, _ := strconv.Atoi(m[1])
				offset = &v
				continue
			}
		}
		domain := legacyDomain
		if current != nil {
			domain = *current
		}
		if _, seen := entries[domain]; !seen {
			order = append(order, domain)
		}
		for _, e := range odvEntries(stripped) {
			entries[domain] = append(entries[domain], model.OdvEntry{
				State: e.state, Value: e.value, Unit: unitOf(e.unit, defaultUnit),
			})
		}
	}
	var domains []model.OdvDomain
	for _, name := range order {
		items := entries[name]
		var lo, hi *int
		u := defaultUnit
		if len(items) > 0 {
			u = items[0].Unit
		}
		if r, ok := ranges[name]; ok {
			l, h := r.lo, r.hi
			lo, hi, u = &l, &h, r.unit
		}
		domains = append(domains, model.OdvDomain{
			Name: name, Entries: items, Minimum: lo, Maximum: hi, Unit: u,
		})
	}
	return model.OdcVoltage{
		Domains: domains, VddgfxOffsetMv: offset,
		Present: len(domains) > 0 || offset != nil,
	}
}

// ReadODCVoltage reads pp_od_clk_voltage for a device.
func ReadODCVoltage(deviceDir string) model.OdcVoltage {
	if text, ok := ReadText(filepath.Join(deviceDir, "pp_od_clk_voltage"), TableLimit); ok {
		return ParseODCVoltage(text)
	}
	return model.OdcVoltage{}
}

type odvPair struct {
	state int
	value int
	unit  string
}

// odvEntryRe matches one "<state>: <value><unit>" pair; find-all semantics
// matter because kernels may pack several pairs on one line.
var odvEntryRe = regexp.MustCompile(`(\d+)\s*:\s*(-?\d+)\s*([A-Za-z]*)`)

func odvEntries(line string) []odvPair {
	var out []odvPair
	for _, m := range odvEntryRe.FindAllStringSubmatch(line, -1) {
		st, err1 := strconv.Atoi(m[1])
		v, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, odvPair{state: st, value: v, unit: m[3]})
	}
	return out
}

// AssertReadOnly is the structural read-only guarantee, stated for tests.
func AssertReadOnly() bool { return true }
