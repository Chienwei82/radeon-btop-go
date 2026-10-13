package readers

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadTextMissing(t *testing.T) {
	if _, ok := ReadText(t.TempDir()+"/nope", ValueLimit); ok {
		t.Fatal("missing file must report absent")
	}
}

func TestReadInt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/v", "42\n")
	if v, ok := ReadInt(dir + "/v"); !ok || v != 42 {
		t.Fatalf("ReadInt = %v,%v want 42,true", v, ok)
	}
	writeFile(t, dir+"/bad", "junk\n")
	if _, ok := ReadInt(dir + "/bad"); ok {
		t.Fatal("garbage must report absent")
	}
}

func TestReadHexInt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/v", "0x1002\n")
	if v, ok := ReadHexInt(dir + "/v"); !ok || v != 0x1002 {
		t.Fatalf("ReadHexInt = %v,%v want 0x1002", v, ok)
	}
}

func TestParseFirstInt(t *testing.T) {
	if v, ok := ParseFirstInt("500\n"); !ok || v != 500 {
		t.Fatalf("ParseFirstInt = %v,%v", v, ok)
	}
	if v, ok := ParseFirstInt("level: 3 (auto)\n"); !ok || v != 3 {
		t.Fatalf("ParseFirstInt embedded = %v,%v", v, ok)
	}
	if _, ok := ParseFirstInt(""); ok {
		t.Fatal("empty must be absent")
	}
	if _, ok := ParseFirstInt("no digits here"); ok {
		t.Fatal("digitless must be absent")
	}
}

func TestParseDrmSize(t *testing.T) {
	if v, ok := ParseDrmSize("4116 KiB"); !ok || v != 4116*1024 {
		t.Fatalf("KiB = %v,%v", v, ok)
	}
	if v, ok := ParseDrmSize("0 B"); !ok || v != 0 {
		t.Fatalf("bytes = %v,%v", v, ok)
	}
	if _, ok := ParseDrmSize("junk"); ok {
		t.Fatal("junk must be absent")
	}
}

func TestParsePowerAndTemp(t *testing.T) {
	if v, ok := ParsePowerMicrowatts("33000000"); !ok || v != 33.0 {
		t.Fatalf("microwatts = %v,%v", v, ok)
	}
	if v, ok := ParseMillidegrees("51000"); !ok || v != 51.0 {
		t.Fatalf("millidegrees = %v,%v", v, ok)
	}
}
