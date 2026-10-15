package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

func profileMetrics() model.AmdgpuMetrics {
	m := testMetrics()
	m.Profiles = model.PowerProfileTable{
		Profiles: []model.PowerProfileEntry{
			{Index: 0, Name: "BOOTUP_DEFAULT", Active: true},
			{Index: 1, Name: "3D_FULL_SCREEN"},
		},
		Present: true,
	}
	return m
}

func odcMetrics() model.AmdgpuMetrics {
	m := testMetrics()
	lo, hi, offset := 500, 2600, -50
	m.ODC = model.OdcVoltage{
		Domains: []model.OdvDomain{{
			Name: "sclk",
			Entries: []model.OdvEntry{
				{State: 0, Value: 500, Unit: model.OdvMegahertz},
				{State: 1, Value: 2104, Unit: model.OdvMegahertz},
			},
			Minimum: &lo, Maximum: &hi, Unit: model.OdvMegahertz,
		}},
		VddgfxOffsetMv: &offset,
		Present:        true,
	}
	return m
}

func profileText(m model.AmdgpuMetrics, width int) string {
	return strings.Join(PlainLines(PowerProfilePanel(m, DefaultTheme, width, nil)), "\n")
}

func TestPowerProfilePanelShowsTheActiveProfile(t *testing.T) {
	if !strings.Contains(profileText(profileMetrics(), 44), "BOOTUP_DEFAULT") {
		t.Fatal("panel misses the active profile")
	}
}

func TestPowerProfilePanelListsAlternativesWithoutOfferingToChangeThem(t *testing.T) {
	text := profileText(profileMetrics(), 44)
	if !strings.Contains(text, "3D_FULL_SCREEN") {
		t.Fatalf("panel = %q, want the alternative profile listed", text)
	}
	if !strings.Contains(text, "read-only") {
		t.Fatalf("panel = %q, want the read-only reminder", text)
	}
}

func TestPowerProfilePanelAbsentTableIsNANotAProfileCalledNone(t *testing.T) {
	text := profileText(testMetrics(), 44)
	if !strings.Contains(text, "profile "+NA) {
		t.Fatalf("panel = %q, want profile N/A", text)
	}
	if !strings.Contains(text, "profiles "+NA) {
		t.Fatalf("panel = %q, want profiles N/A", text)
	}
}

func TestPowerProfilePanelOverdriveDomainsKeepTheirOwnUnits(t *testing.T) {
	text := profileText(odcMetrics(), 44)
	if !strings.Contains(text, "2104/2600 MHz") {
		t.Fatalf("panel = %q, want the clock ceiling in MHz", text)
	}
	if !strings.Contains(text, "-50 mV") {
		t.Fatalf("panel = %q, want the vddgfx offset", text)
	}
}

func TestPowerProfilePanelLegacyMillivoltTableIsNotReportedAsMegahertz(t *testing.T) {
	lo, hi := 300, 450
	m := testMetrics()
	m.ODC = model.OdcVoltage{
		Domains: []model.OdvDomain{{
			Name: "sclk",
			Entries: []model.OdvEntry{
				{State: 0, Value: 300, Unit: model.OdvMillivolt},
				{State: 1, Value: 450, Unit: model.OdvMillivolt},
			},
			Minimum: &lo, Maximum: &hi, Unit: model.OdvMillivolt,
		}},
		Present: true,
	}
	if text := profileText(m, 44); !strings.Contains(text, "450/450 mV") {
		t.Fatalf("panel = %q, want the legacy unit preserved", text)
	}
}

func TestPowerProfilePanelAbsentOverdriveIsNA(t *testing.T) {
	if text := profileText(testMetrics(), 44); !strings.Contains(text, "overdrive "+NA) {
		t.Fatalf("panel = %q, want overdrive N/A", text)
	}
}

func TestPowerProfilePanelOverdriveRowsFitTheAvailableWidth(t *testing.T) {
	for _, width := range []int{44, 58, 80} {
		for _, row := range PlainLines(PowerProfilePanel(odcMetrics(), DefaultTheme, width, nil)) {
			if !strings.HasPrefix(row, ODPrefix) {
				continue
			}
			if n := runeLen(row); n > width {
				t.Fatalf("width %d: overdrive row is %d cells: %q", width, n, row)
			}
		}
	}
}

func TestPowerProfilePanelTrimsDetailRowsInsteadOfWrapping(t *testing.T) {
	entries := make([]model.PowerProfileEntry, 0, 12)
	for i := range 12 {
		entries = append(entries, model.PowerProfileEntry{Index: i, Name: "AN_EXTREMELY_LONG_PROFILE_NAME_" + strconv.Itoa(i)})
	}
	m := testMetrics()
	m.Profiles = model.PowerProfileTable{Profiles: entries, Present: true}
	rows := PlainLines(PowerProfilePanel(m, DefaultTheme, 44, nil))
	available := ""
	for _, row := range rows {
		if strings.Contains(row, "available") {
			available = row
		}
	}
	if available == "" {
		t.Fatalf("rows = %q, want an available row", rows)
	}
	if !strings.HasSuffix(available, "…") {
		t.Fatalf("available row = %q, want a truncation marker", available)
	}
	if n := runeLen(available); n > 44 {
		t.Fatalf("available row is %d cells: %q", n, available)
	}
	for _, row := range rows {
		if n := runeLen(row); n > 44 {
			t.Fatalf("row is %d cells, want at most 44: %q", n, row)
		}
	}
}

func TestPowerProfilePanelStartsWithTheProfileAndEndsWithReadOnly(t *testing.T) {
	rows := PlainLines(PowerProfilePanel(profileMetrics(), DefaultTheme, 44, nil))
	if !strings.Contains(rows[0], "profile") {
		t.Fatalf("first row = %q, want the profile", rows[0])
	}
	if !strings.Contains(rows[len(rows)-1], "read-only") {
		t.Fatalf("last row = %q, want the read-only reminder", rows[len(rows)-1])
	}
}
