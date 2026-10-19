package sessionlog

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
	"github.com/klauspost/compress/zstd"
)

// wallClockNS is a fixed stamp so the time column is reproducible.
const wallClockNS = 1_700_000_000_000_000_000

// zstdMagic starts every zstd frame; anything else is not a zstd file.
var zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}

const mib = 1024 * 1024

func testDevice() model.AmdgpuDevice {
	return model.AmdgpuDevice{
		Index: 0, Card: "card0", BDF: "0000:0c:00.0",
		VendorID: 0x1002, DeviceID: 0x73BF,
		DeviceDir: "/sys/devices/fake", Kind: model.DeviceDGPU,
		Name: "AMD Radeon RX 6800",
	}
}

// bareMetrics reports nothing at all: every optional field is nil.
func bareMetrics() model.AmdgpuMetrics {
	return model.AmdgpuMetrics{Device: testDevice(), Sequence: 1, IntervalS: 1.0}
}

// richMetrics reports a plausible reading, enough to pin every column that
// carries a value.
func richMetrics() model.AmdgpuMetrics {
	gpu, mem := 42.5, 17.25
	sclk, mclk, soc := 2200, 1000, 800
	vramUsed, vramTotal := model.Bytes(2*mib), model.Bytes(16*mib)
	draw, cap := 45.0, 211.0
	rpm := 2400
	edge, junction := 49.0, 56.0
	profile := "BOOTUP_DEFAULT"
	blockGpu, blockVgt := 77.0, 12.0
	return model.AmdgpuMetrics{
		Device:         testDevice(),
		Sequence:       7,
		IntervalS:      1.0,
		GpuBusyPercent: &gpu,
		MemBusyPercent: &mem,
		VRAM:           model.MemoryPool{Used: &vramUsed, Total: &vramTotal},
		SCLK:           model.Clock{Current: &sclk},
		MCLK:           model.Clock{Current: &mclk},
		SocClk:         model.Clock{Current: &soc},
		Power:          model.Power{DrawW: &draw, CapW: &cap},
		Fan:            model.Fan{RPM: &rpm},
		Temperatures: []model.Temperature{
			{Label: "edge", Celsius: &edge},
			{Label: "junction", Celsius: &junction},
		},
		PowerProfile: &profile,
		Throttle:     &model.ThrottleInfo{Active: []string{"TEMP_GPU", "SW_THERMAL"}},
		ODC: model.OdcVoltage{Domains: []model.OdvDomain{
			{Name: "sclk", Unit: model.OdvMegahertz, Entries: []model.OdvEntry{{State: 1, Value: 2104}}},
		}},
		Blocks: &model.GpuBlocks{
			Blocks: []model.BlockReading{
				{Key: "gpu", Label: "Graphics pipe", Percent: blockGpu},
				{Key: "vgt", Label: "Vertex Grouper", Percent: blockVgt},
			},
		},
	}
}

func testSnapshot(devices ...model.AmdgpuMetrics) model.GpuSnapshot {
	return model.GpuSnapshot{
		Sequence: 1, TimestampNs: 1_000_000,
		Devices: devices, WallClockNs: wallClockNS + 1,
	}
}

// readRows parses a recorded CSV the way a consumer would.
func readRows(t *testing.T, path string) [][]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return rows
}

func cellOf(t *testing.T, header, row []string, column string) string {
	t.Helper()
	for i, name := range header {
		if name == column {
			return row[i]
		}
	}
	t.Fatalf("column %q not in header", column)
	return ""
}

// decompress reads a zstd file with the library's own reader, which proves it
// is a real frame and not merely a file with a suggestive name.
func decompress(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder, err := zstd.NewReader(bytes.NewReader(raw), zstd.WithDecoderConcurrency(1))
	if err != nil {
		t.Fatalf("decoder: %v", err)
	}
	defer decoder.Close()
	out, err := io.ReadAll(decoder)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	return out
}

func TestSplitTarget(t *testing.T) {
	cases := []struct {
		name       string
		format     Format
		compressed bool
		stem       string
	}{
		{"s.csv", FormatCSV, false, "s.csv"},
		{"s.json", FormatJSONL, false, "s.json"},
		{"s.jsonl", FormatJSONL, false, "s.jsonl"},
		{"s.ndjson", FormatJSONL, false, "s.ndjson"},
		{"s.csv.zst", FormatCSV, true, "s.csv"},
		{"s.json.zst", FormatJSONL, true, "s.json"},
		{"s.ndjson.zstd", FormatJSONL, true, "s.ndjson"},
		{"S.CSV.ZST", FormatCSV, true, "S.CSV"},
		// Only the basename is parsed: a directory named logs.csv must not
		// decide the format.
		{"/var/log/session.json", FormatJSONL, false, "session.json"},
	}
	for _, c := range cases {
		format, compressed, stem, err := SplitTarget(c.name)
		if err != nil {
			t.Errorf("SplitTarget(%q) error = %v", c.name, err)
			continue
		}
		if format != c.format || compressed != c.compressed || stem != c.stem {
			t.Errorf("SplitTarget(%q) = (%v, %v, %q), want (%v, %v, %q)",
				c.name, format, compressed, stem, c.format, c.compressed, c.stem)
		}
	}
}

func TestSplitTargetRejectsAnUnusableSuffix(t *testing.T) {
	for _, name := range []string{"s.txt", "s.log", "s", "s.tsv"} {
		if _, _, _, err := SplitTarget(name); err == nil {
			t.Errorf("SplitTarget(%q) accepted an unusable suffix", name)
		}
	}
}

func TestCSVHeaderIsWrittenExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.csv")
	log := OpenLog(path, 0)
	if !log.IsOpen() {
		t.Fatalf("log not open: %v %s", log.Problem, log.Detail)
	}
	for sequence := 1; sequence <= 3; sequence++ {
		metrics := bareMetrics()
		metrics.Sequence = sequence
		snapshot := testSnapshot(metrics)
		snapshot.Sequence = sequence
		if !log.Write(snapshot) {
			t.Fatalf("write %d failed: %v %s", sequence, log.Problem, log.Detail)
		}
	}
	log.Close()
	lines, err := ReadLog(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want 4: %q", len(lines), lines)
	}
	header := lines[0]
	count := 0
	for _, line := range lines {
		if line == header {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("header appears %d times, want 1", count)
	}
	if !bytes.HasPrefix([]byte(header), []byte("time,sequence,device_index,")) {
		t.Fatalf("header = %q", header)
	}
}

func TestCSVHeaderNamesEveryColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.csv")
	log := OpenLog(path, 0)
	log.Write(testSnapshot(bareMetrics()))
	log.Close()
	rows := readRows(t, path)
	if len(rows[0]) != len(CSVColumns) {
		t.Fatalf("header has %d cells, CSVColumns has %d", len(rows[0]), len(CSVColumns))
	}
	for i, want := range CSVColumns {
		if rows[0][i] != want {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], want)
		}
	}
}

func TestCSVEveryRowHasTheFullColumnCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.csv")
	log := OpenLog(path, 0)
	log.Write(testSnapshot(richMetrics(), bareMetrics()))
	log.Close()
	rows := readRows(t, path)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	for _, row := range rows {
		if len(row) != len(CSVColumns) {
			t.Fatalf("row has %d cells, want %d: %v", len(row), len(CSVColumns), row)
		}
	}
}

// measurementColumns are the columns that carry a reading and must be empty
// for a device that reported none.
var measurementColumns = []string{
	"gpu_percent", "mem_percent", "vram_used", "vram_total", "gtt_used",
	"gtt_total", "sclk_mhz", "mclk_mhz", "socclk_mhz", "power_w",
	"power_cap_w", "fan_rpm", "odc_sclk", "odc_mclk", "throttling",
	"temp_edge", "temp_junction", "temp_mem", "temp_soc", "temp_core",
	"temp_l3", "block_gpu", "block_ee",
}

func TestCSVAbsentReadingIsAnEmptyCell(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.csv")
	log := OpenLog(path, 0)
	if !log.Write(testSnapshot(bareMetrics())) {
		t.Fatalf("write failed: %v %s", log.Problem, log.Detail)
	}
	log.Close()
	rows := readRows(t, path)
	for _, column := range measurementColumns {
		if got := cellOf(t, rows[0], rows[1], column); got != "" {
			t.Errorf("%s = %q, want an empty cell", column, got)
		}
	}
	for _, cell := range rows[1] {
		if cell == "N/A" {
			t.Errorf("row contains N/A: %v", rows[1])
		}
	}
}

func TestCSVPresentReadingAppears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "full.csv")
	log := OpenLog(path, 0)
	log.Write(testSnapshot(richMetrics()))
	log.Close()
	rows := readRows(t, path)
	cases := map[string]string{
		"sequence": "7", "card": "card0", "bdf": "0000:0c:00.0",
		"name": "AMD Radeon RX 6800", "gpu_percent": "42.50",
		"mem_percent": "17.25", "vram_used": "2097152",
		"vram_total": "16777216", "sclk_mhz": "2200.00",
		"power_w": "45.0", "power_cap_w": "211.0", "fan_rpm": "2400",
		"profile": "BOOTUP_DEFAULT", "temp_edge": "49.00", "block_gpu": "77.00",
	}
	for column, want := range cases {
		if got := cellOf(t, rows[0], rows[1], column); got != want {
			t.Errorf("%s = %q, want %q", column, got, want)
		}
	}
}

func TestCSVZeroIsAMeasurementNotAnAbsence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zero.csv")
	zero, rpm := 0.0, 0
	metrics := bareMetrics()
	metrics.GpuBusyPercent = &zero
	metrics.Fan.RPM = &rpm
	log := OpenLog(path, 0)
	log.Write(testSnapshot(metrics))
	log.Close()
	rows := readRows(t, path)
	if got := cellOf(t, rows[0], rows[1], "gpu_percent"); got != "0.00" {
		t.Errorf("gpu_percent = %q, want 0.00", got)
	}
	if got := cellOf(t, rows[0], rows[1], "fan_rpm"); got != "0" {
		t.Errorf("fan_rpm = %q, want 0", got)
	}
	if got := cellOf(t, rows[0], rows[1], "power_w"); got != "" {
		t.Errorf("power_w = %q, want empty next to the real zeros", got)
	}
}

func TestCSVQuotesAThrottleReasonList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "throttled.csv")
	log := OpenLog(path, 0)
	log.Write(testSnapshot(richMetrics()))
	log.Close()
	lines, err := ReadLog(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !containsText(lines[1], `"SW_THERMAL,TEMP_GPU"`) {
		t.Fatalf("row = %q, want a quoted, sorted reason list", lines[1])
	}
}

func containsText(haystack, needle string) bool {
	return len(haystack) >= len(needle) && bytes.Contains([]byte(haystack), []byte(needle))
}

func TestJSONLRecordRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	log := OpenLog(path, 0)
	log.Write(testSnapshot(richMetrics()))
	log.Close()
	lines, err := ReadLog(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(lines))
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode: %v", err)
	}
	device, ok := record["device"].(map[string]any)
	if !ok {
		t.Fatalf("device = %#v", record["device"])
	}
	if device["card"] != "card0" || device["bdf"] != "0000:0c:00.0" ||
		device["name"] != "AMD Radeon RX 6800" || device["integrated"] != false {
		t.Errorf("device = %#v", device)
	}
	if record["gpu_percent"] != 42.5 {
		t.Errorf("gpu_percent = %#v, want 42.5", record["gpu_percent"])
	}
	// The record carries the snapshot's sequence, the CSV its per-device one.
	if record["sequence"] != float64(1) {
		t.Errorf("sequence = %#v, want the snapshot sequence 1", record["sequence"])
	}
	clocks, ok := record["clocks_mhz"].(map[string]any)
	if !ok || clocks["sclk"] != 2200.0 || clocks["mclk"] != 1000.0 || clocks["socclk"] != 800.0 {
		t.Errorf("clocks_mhz = %#v", record["clocks_mhz"])
	}
	temps, ok := record["temperatures_c"].(map[string]any)
	if !ok || temps["edge"] != 49.0 || temps["junction"] != 56.0 {
		t.Errorf("temperatures_c = %#v", record["temperatures_c"])
	}
	throttling, ok := record["throttling"].([]any)
	if !ok || len(throttling) != 2 || throttling[0] != "SW_THERMAL" || throttling[1] != "TEMP_GPU" {
		t.Errorf("throttling = %#v, want sorted [SW_THERMAL TEMP_GPU]", record["throttling"])
	}
	blocks, ok := record["blocks"].(map[string]any)
	if !ok || blocks["busiest"] != "gpu" {
		t.Fatalf("blocks = %#v", record["blocks"])
	}
	percent, ok := blocks["percent"].(map[string]any)
	if !ok || percent["gpu"] != 77.0 || percent["vgt"] != 12.0 {
		t.Errorf("blocks.percent = %#v", blocks["percent"])
	}
	if record["profile"] != "BOOTUP_DEFAULT" {
		t.Errorf("profile = %#v", record["profile"])
	}
	if record["time"] != "2023-11-14T22:13:20.000+00:00" {
		t.Errorf("time = %#v, want the fixed ISO stamp", record["time"])
	}
	if record["monotonic_ns"] != float64(1_000_000) {
		t.Errorf("monotonic_ns = %#v", record["monotonic_ns"])
	}
}

func TestJSONLAnAbsentReadingIsNull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.json")
	log := OpenLog(path, 0)
	log.Write(testSnapshot(bareMetrics()))
	log.Close()
	lines, _ := ReadLog(path)
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"gpu_percent", "mem_percent", "fan_rpm", "profile", "throttling", "blocks"} {
		if value, present := record[key]; !present || value != nil {
			t.Errorf("%s = %#v, want null", key, record[key])
		}
	}
	for _, key := range []string{"vram", "gtt"} {
		pool, ok := record[key].(map[string]any)
		if !ok {
			t.Fatalf("%s = %#v", key, record[key])
		}
		for _, member := range []string{"used", "total", "percent"} {
			if pool[member] != nil {
				t.Errorf("%s.%s = %#v, want null", key, member, pool[member])
			}
		}
	}
	clocks, _ := record["clocks_mhz"].(map[string]any)
	for _, member := range []string{"sclk", "mclk", "socclk"} {
		if clocks[member] != nil {
			t.Errorf("clocks_mhz.%s = %#v, want null", member, clocks[member])
		}
	}
	temps, ok := record["temperatures_c"].(map[string]any)
	if !ok || len(temps) != 0 {
		t.Errorf("temperatures_c = %#v, want {}", record["temperatures_c"])
	}
}

func TestJSONLHasNoHeaderAndOneLinePerSample(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.ndjson")
	log := OpenLog(path, 0)
	log.Write(testSnapshot(bareMetrics()))
	second := bareMetrics()
	second.Sequence = 2
	snapshot := testSnapshot(second)
	snapshot.Sequence = 2
	log.Write(snapshot)
	log.Close()
	lines, _ := ReadLog(path)
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	for i, line := range lines {
		if !bytes.HasPrefix([]byte(line), []byte("{")) {
			t.Fatalf("line %d = %q, want a JSON object", i, line)
		}
	}
}

func TestZstdStartsWithTheFrameMagic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.csv.zst")
	log := OpenLog(path, 0)
	log.Write(testSnapshot(bareMetrics()))
	log.Close()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.HasPrefix(raw, zstdMagic) {
		t.Fatalf("file starts with % x, want the zstd magic", raw[:4])
	}
}

func TestZstdBytesDecompressToThePlainText(t *testing.T) {
	plain := filepath.Join(t.TempDir(), "plain.csv")
	packed := filepath.Join(t.TempDir(), "packed.csv.zst")
	for _, path := range []string{plain, packed} {
		log := OpenLog(path, 0)
		log.Write(testSnapshot(richMetrics()))
		log.Close()
	}
	raw, err := os.ReadFile(plain)
	if err != nil {
		t.Fatalf("read plain: %v", err)
	}
	if got := decompress(t, packed); !bytes.Equal(got, raw) {
		t.Fatalf("decompressed bytes differ from the plain recording")
	}
}

func TestReadLogDecompressesTransparently(t *testing.T) {
	plain := filepath.Join(t.TempDir(), "plain.json")
	packed := filepath.Join(t.TempDir(), "packed.json.zstd")
	for _, path := range []string{plain, packed} {
		log := OpenLog(path, 0)
		for sequence := 1; sequence <= 3; sequence++ {
			metrics := richMetrics()
			metrics.Sequence = sequence
			snapshot := testSnapshot(metrics)
			snapshot.Sequence = sequence
			log.Write(snapshot)
		}
		log.Close()
	}
	plainLines, err := ReadLog(plain)
	if err != nil {
		t.Fatalf("plain: %v", err)
	}
	packedLines, err := ReadLog(packed)
	if err != nil {
		t.Fatalf("packed: %v", err)
	}
	if len(packedLines) != len(plainLines) || len(packedLines) != 3 {
		t.Fatalf("packed lines = %d, plain lines = %d, want 3 each", len(packedLines), len(plainLines))
	}
}

func TestReopeningAppendsRatherThanTruncating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.csv")
	first := OpenLog(path, 0)
	first.Write(testSnapshot(bareMetrics()))
	first.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	second := OpenLog(path, 0)
	if !second.HeaderWritten {
		t.Fatal("reopened log does not know the header exists")
	}
	metrics := bareMetrics()
	metrics.Sequence = 2
	snapshot := testSnapshot(metrics)
	snapshot.Sequence = 2
	second.Write(snapshot)
	second.Close()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.HasPrefix(after, before) {
		t.Fatal("reopening truncated the earlier recording")
	}
	rows := readRows(t, path)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (one header, two records)", len(rows))
	}
	if rows[0][0] != "time" {
		t.Errorf("first cell = %q, want the header", rows[0][0])
	}
}

func TestAnExistingButEmptyFileStillGetsAHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.csv")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	log := OpenLog(path, 0)
	if log.HeaderWritten {
		t.Fatal("an empty file must not count as having a header")
	}
	log.Write(testSnapshot(bareMetrics()))
	log.Close()
	lines, _ := ReadLog(path)
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if !bytes.HasPrefix([]byte(lines[0]), []byte("time,sequence")) {
		t.Fatalf("first line = %q, want the header", lines[0])
	}
}

func TestAnUnknownSuffixIsReportedNotRaised(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.txt")
	log := OpenLog(path, 0)
	if log.Problem != ProblemUnknownSuffix {
		t.Fatalf("problem = %q, want %q", log.Problem, ProblemUnknownSuffix)
	}
	if log.IsOpen() {
		t.Fatal("a log with an unknown suffix must not be open")
	}
	if log.Write(testSnapshot(bareMetrics())) {
		t.Fatal("write on an unusable log reported success")
	}
	if log.Records() != 0 {
		t.Fatalf("records = %d, want 0", log.Records())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("an unknown suffix must not create the file")
	}
}

func TestAnUnopenablePathIsReportedNotRaised(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	log := OpenLog(filepath.Join(blocker, "session.csv"), 0)
	if log.Problem != ProblemOpenFailed {
		t.Fatalf("problem = %q, want %q", log.Problem, ProblemOpenFailed)
	}
	if log.IsOpen() {
		t.Fatal("unopenable log reports itself open")
	}
	if log.Write(testSnapshot(bareMetrics())) {
		t.Fatal("write on an unopenable log reported success")
	}
	log.Close() // must be safe on a log that never opened
}

func TestAnOutOfRangeLevelIsReported(t *testing.T) {
	log := OpenLog(filepath.Join(t.TempDir(), "s.csv.zst"), maxZstdLevel+1)
	if log.Problem != ProblemOpenFailed {
		t.Fatalf("problem = %q, want %q", log.Problem, ProblemOpenFailed)
	}
	if log.IsOpen() {
		t.Fatal("out-of-range level left the log open")
	}
}

func TestEveryValidLevelRoundTrips(t *testing.T) {
	for _, level := range []int{0, 1, 3, 9, 19, 22} {
		path := filepath.Join(t.TempDir(), "level.csv.zst")
		log := OpenLog(path, level)
		if !log.IsOpen() {
			t.Fatalf("level %d: not open (%v: %s)", level, log.Problem, log.Detail)
		}
		log.Write(testSnapshot(bareMetrics()))
		log.Close()
		lines, err := ReadLog(path)
		if err != nil {
			t.Fatalf("level %d: read: %v", level, err)
		}
		if len(lines) != 2 {
			t.Fatalf("level %d: lines = %d, want 2", level, len(lines))
		}
	}
}

func TestAnEmptySnapshotIsNotAFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.csv")
	log := OpenLog(path, 0)
	if !log.Write(model.GpuSnapshot{}) {
		t.Fatal("an empty snapshot reported the log dead")
	}
	if log.Records() != 0 {
		t.Fatalf("records = %d, want 0", log.Records())
	}
	if !log.Write(testSnapshot(bareMetrics())) {
		t.Fatal("the log stopped working after an empty snapshot")
	}
	log.Close()
	if log.Records() != 1 {
		t.Fatalf("records = %d, want 1", log.Records())
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	log := OpenLog(filepath.Join(t.TempDir(), "s.csv.zst"), 0)
	log.Write(testSnapshot(bareMetrics()))
	log.Close()
	log.Close() // must not panic or corrupt anything
}
