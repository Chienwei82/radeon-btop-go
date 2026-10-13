package readers

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAttr(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDPMStarredRow(t *testing.T) {
	cur, max := ParseDPMTable("0: 500Mhz\n1: 2104Mhz *\n")
	if cur == nil || *cur != 2104 || max == nil || *max != 2104 {
		t.Fatalf("starred = %v,%v", cur, max)
	}
}

func TestDPMUnmarkedUsesFirst(t *testing.T) {
	cur, max := ParseDPMTable("0: 300Mhz\n1: 900Mhz\n")
	if cur == nil || *cur != 300 || max == nil || *max != 900 {
		t.Fatalf("unmarked = %v,%v", cur, max)
	}
}

func TestDPMEmpty(t *testing.T) {
	if cur, max := ParseDPMTable(""); cur != nil || max != nil {
		t.Fatal("empty must be (nil,nil)")
	}
}

func TestReadClockFallback(t *testing.T) {
	dir := t.TempDir()
	writeAttr(t, dir+"/pp_dpm_sclk", "0: 500Mhz\n1: 900Mhz *\n")
	cur, conf := ReadClock(dir, "current_sclk", "pp_dpm_sclk")
	if cur == nil || *cur != 900 || conf != 1 {
		t.Fatalf("dpm fallback = %v,%d", cur, conf)
	}
	writeAttr(t, dir+"/current_sclk", "1200\n")
	cur, conf = ReadClock(dir, "current_sclk", "pp_dpm_sclk")
	if cur == nil || *cur != 1200 || conf != 2 {
		t.Fatalf("direct = %v,%d", cur, conf)
	}
}

func TestLinkSpeed(t *testing.T) {
	v := ParseLinkSpeed("16.0 GT/s PCIe\n")
	if v == nil || *v != 160 {
		t.Fatalf("link speed = %v", v)
	}
	if ParseLinkSpeed("") != nil {
		t.Fatal("empty must be nil")
	}
}

func TestPowerProfile(t *testing.T) {
	text := "0 BOOTUP_DEFAULT\n1 3D_FULL_SCREEN*\n"
	if got := ParsePowerProfile(text); got == nil || *got != "3D_FULL_SCREEN" {
		t.Fatalf("profile = %v", got)
	}
	if got := ParsePowerProfile("0 BOOTUP_DEFAULT\n"); got == nil || *got != "BOOTUP_DEFAULT" {
		t.Fatalf("fallback = %v", got)
	}
}

func TestHwmonReads(t *testing.T) {
	dir := t.TempDir()
	writeAttr(t, dir+"/temp1_input", "51000\n")
	writeAttr(t, dir+"/temp1_label", "edge\n")
	writeAttr(t, dir+"/temp2_input", "58000\n")
	writeAttr(t, dir+"/temp2_label", "junction\n")
	writeAttr(t, dir+"/power1_average", "33000000\n")
	writeAttr(t, dir+"/power1_cap", "200000000\n")
	writeAttr(t, dir+"/fan1_input", "1200\n")
	writeAttr(t, dir+"/fan1_max", "3000\n")
	got := ReadTemperatures(dir)
	if got["edge"] != 51 || got["junction"] != 58 {
		t.Fatalf("temps = %v", got)
	}
	draw, cap := ReadPower(dir)
	if draw == nil || *draw != 33 || cap == nil || *cap != 200 {
		t.Fatalf("power = %v,%v", draw, cap)
	}
	rpm, maxRPM, _ := ReadFan(dir)
	if rpm == nil || *rpm != 1200 || maxRPM == nil || *maxRPM != 3000 {
		t.Fatalf("fan = %v,%v", rpm, maxRPM)
	}
	if len(ReadTemperatures("")) != 0 {
		t.Fatal("empty hwmon dir must be empty, not error")
	}
}
