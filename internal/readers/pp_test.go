package readers

import (
	"strings"
	"testing"
)

// rdna2ODC is the real pp_od_clk_voltage of a Navi 21, verbatim from Python.
const rdna2ODC = "OD_SCLK:\n0: 500Mhz\n1: 2104Mhz\nOD_MCLK:\n0: 97Mhz\n1: 1000MHz\nOD_RANGE:\nSCLK:     500Mhz       2600Mhz\nMCLK:     674Mhz       1075Mhz\nOD_VDDGFX_OFFSET:\n-50mV\n"

// legacyODC is the legacy single voltage table, verbatim from Python.
const legacyODC = "0: 300\n1: 350\n2: 400\n3: 450\n"

// realProfiles is the real pp_power_profile_mode, verbatim from Python.
const realProfiles = "PROFILE_INDEX(NAME) CLOCK_TYPE(NAME) FPS MinFreqType MinActiveFreqType " +
	"MinActiveFreq BoosterFreqType BoosterFreq PD_Data_limit_c\n" +
	" 0 BOOTUP_DEFAULT*:\n" +
	"                     0(       GFXCLK)       0       5       1       0       4     800 4587520\n" +
	"                     1(      SOCCLK)       0       5       1       0       1       0 3276800\n" +
	" 1 3D_FULL_SCREEN :\n" +
	"                     0(       GFXCLK)       0       5       0    1600       4     650 5242880\n" +
	" 2   POWER_SAVING :\n" +
	" 3          VIDEO :\n" +
	" 4             VR :\n" +
	" 5         COMPUTE :\n" +
	" 6          CUSTOM :\n"

func TestPowerProfileRealFile(t *testing.T) {
	got := ParsePowerProfileTable(realProfiles)
	if !got.Present || len(got.Profiles) != 7 {
		t.Fatalf("profiles = %+v", got)
	}
	if got.ActiveName() != "BOOTUP_DEFAULT" {
		t.Fatalf("active = %q", got.ActiveName())
	}
	if got.Describe() != "BOOTUP_DEFAULT (0 of 7)" {
		t.Fatalf("describe = %q", got.Describe())
	}
	for _, p := range got.Profiles {
		if strings.HasPrefix(p.Name, "(") {
			t.Fatalf("detail row leaked: %+v", p)
		}
	}
}

func TestPowerProfileOtherActive(t *testing.T) {
	got := ParsePowerProfileTable(" 0 BOOTUP_DEFAULT :\n 1 3D_FULL_SCREEN*:\n")
	if got.ActiveName() != "3D_FULL_SCREEN" {
		t.Fatalf("active = %q", got.ActiveName())
	}
}

func TestODCModernEras(t *testing.T) {
	got := ParseODCVoltage(rdna2ODC)
	if !got.Present {
		t.Fatal("modern table must be present")
	}
	sclk := got.Domain("sclk")
	if sclk == nil || sclk.Ceiling() == nil || *sclk.Ceiling() != 2104 {
		t.Fatalf("sclk = %+v", sclk)
	}
	if sclk.Describe() != "2104/2600 MHz" {
		t.Fatalf("describe = %q", sclk.Describe())
	}
	mclk := got.Domain("mclk")
	if mclk == nil || mclk.Describe() != "1000/1075 MHz" {
		t.Fatalf("mclk = %+v", mclk)
	}
	if got.VddgfxOffsetMv == nil || *got.VddgfxOffsetMv != -50 {
		t.Fatalf("offset = %v", got.VddgfxOffsetMv)
	}
}

func TestODCLegacyVoltage(t *testing.T) {
	got := ParseODCVoltage(legacyODC)
	if !got.Present {
		t.Fatal("legacy table must be present")
	}
	legacy := got.Domain("legacy")
	if legacy == nil || legacy.Unit != "mV" {
		t.Fatalf("legacy = %+v", legacy)
	}
	if legacy.Ceiling() == nil || *legacy.Ceiling() != 450 {
		t.Fatalf("ceiling = %v", legacy.Ceiling())
	}
	if legacy.Describe() != "450 mV" {
		t.Fatalf("describe = %q", legacy.Describe())
	}
}

func TestODCOneLinePairs(t *testing.T) {
	got := ParseODCVoltage("0: 300 1: 350 2: 400\n")
	d := got.Domain("legacy")
	if d == nil || len(d.Entries) != 3 {
		t.Fatalf("entries = %+v", d)
	}
}

func TestPPMissingFile(t *testing.T) {
	dir := t.TempDir()
	if got := ReadPowerProfileTable(dir); got.Present {
		t.Fatal("missing profile table must be absent")
	}
	if got := ReadODCVoltage(dir); got.Present {
		t.Fatal("missing ODC table must be absent")
	}
}

// TestODCDomainsKeepFileOrder guards the parity contract of the dump: the
// reference emits one domain per clock the driver listed, in *file* order, and
// an alphabetical sort put OD_MCLK before OD_SCLK. The read-only scanner that
// used to live here moved to readonly_test.go, which checks the AST rather than
// a handful of substrings.
func TestODCDomainsKeepFileOrder(t *testing.T) {
	got := ParseODCVoltage(rdna2ODC)
	if len(got.Domains) != 2 {
		t.Fatalf("domains = %+v", got.Domains)
	}
	if got.Domains[0].Name != "sclk" || got.Domains[1].Name != "mclk" {
		t.Fatalf("domain order = %s,%s, want sclk,mclk",
			got.Domains[0].Name, got.Domains[1].Name)
	}
}
