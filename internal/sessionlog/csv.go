package sessionlog

import (
	"strconv"
	"strings"
	"time"

	"github.com/gputop/gputop-go/internal/model"
)

// csvBaseColumns are the fixed columns, in the order a reader sees them.
var csvBaseColumns = []string{
	"time",
	"sequence",
	"device_index",
	"card",
	"bdf",
	"name",
	"gpu_percent",
	"mem_percent",
	"vram_used",
	"vram_total",
	"gtt_used",
	"gtt_total",
	"sclk_mhz",
	"mclk_mhz",
	"socclk_mhz",
	"power_w",
	"power_cap_w",
	"fan_rpm",
	"profile",
	"odc_sclk",
	"odc_mclk",
	"throttling",
}

// csvTempLabels are the temperature columns, in a fixed order. A label the
// card does not report leaves its cell empty rather than shifting the rest.
var csvTempLabels = []string{"edge", "junction", "mem", "soc", "core", "l3"}

// CSVColumns is the CSV header, built once so the column order and the row
// builder cannot drift.
//
// Block columns come from model.BlockOrder and are present whether or not
// radeontop is running: a card with no Texture Cache block leaves that cell
// empty instead of moving every later column. A stable column set is the
// whole point of the header -- a consumer reads row 4000 with the same code
// that read row 1, including in a session where radeontop was installed
// halfway through.
var CSVColumns = buildCSVColumns()

func buildCSVColumns() []string {
	columns := make([]string, 0, len(csvBaseColumns)+len(csvTempLabels)+len(model.BlockOrder))
	columns = append(columns, csvBaseColumns...)
	for _, label := range csvTempLabels {
		columns = append(columns, "temp_"+label)
	}
	for _, key := range model.BlockOrder {
		columns = append(columns, "block_"+key)
	}
	return columns
}

// csvRow flattens one device's sample into cells, in CSVColumns order.
//
// An absent reading is an empty cell, never 0 and never "N/A": every CSV
// reader treats empty as "no value", whereas a literal N/A in a numeric
// column is a parse error the consumer has to special-case.
func csvRow(snapshot model.GpuSnapshot, metrics *model.AmdgpuMetrics) []string {
	temps := make(map[string]*model.Celsius, len(metrics.Temperatures))
	for _, t := range metrics.Temperatures {
		temps[t.Label] = t.Celsius
	}
	blocks := map[string]model.Percent{}
	if metrics.Blocks != nil {
		blocks = metrics.Blocks.ByKey()
	}

	throttling := ""
	if metrics.Throttle != nil {
		active := append([]string(nil), metrics.Throttle.Active...)
		sortStrings(active)
		throttling = strings.Join(active, ",")
	}

	cells := []string{
		isoTime(snapshot),
		strconv.Itoa(metrics.Sequence),
		strconv.Itoa(metrics.Device.Index),
		metrics.Device.Card,
		metrics.Device.BDF,
		metrics.Device.Name,
		csvNumber(metrics.GpuBusyPercent, 2),
		csvNumber(metrics.MemBusyPercent, 2),
		csvInteger(metrics.VRAM.Used),
		csvInteger(metrics.VRAM.Total),
		csvInteger(metrics.GTT.Used),
		csvInteger(metrics.GTT.Total),
		csvNumber(metrics.SCLK.Current, 2),
		csvNumber(metrics.MCLK.Current, 2),
		csvNumber(metrics.SocClk.Current, 2),
		csvNumber(metrics.Power.DrawW, 1),
		csvNumber(metrics.Power.CapW, 1),
		csvInteger(metrics.Fan.RPM),
		profileName(metrics),
		odcSummary(metrics.ODC, "sclk"),
		odcSummary(metrics.ODC, "mclk"),
		throttling,
	}
	for _, label := range csvTempLabels {
		cells = append(cells, csvNumber(temps[label], 2))
	}
	for _, key := range model.BlockOrder {
		if pct, ok := blocks[key]; ok {
			cells = append(cells, csvNumber(&pct, 2))
			continue
		}
		cells = append(cells, "")
	}
	return cells
}

// numeric is the set of reading types the CSV writer formats.
type numeric interface {
	~int | ~int64 | ~float64
}

// csvNumber formats an optional reading. nil is an empty cell, never 0.
func csvNumber[T numeric](value *T, digits int) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(float64(*value), 'f', digits, 64)
}

// csvInteger formats an optional integer reading. nil is an empty cell.
func csvInteger[T numeric](value *T) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(int64(*value), 10)
}

// profileName is the SCPP profile, falling back to the legacy performance
// level. Both nil means the cell is empty.
func profileName(metrics *model.AmdgpuMetrics) string {
	if metrics.PowerProfile != nil && *metrics.PowerProfile != "" {
		return *metrics.PowerProfile
	}
	if metrics.PerformanceLevel != nil {
		return *metrics.PerformanceLevel
	}
	return ""
}

// odcSummary is one overdrive domain's summary, or an empty cell when the card
// reports no such domain.
func odcSummary(odc model.OdcVoltage, domain string) string {
	if entry := odc.Domain(domain); entry != nil {
		return entry.Describe()
	}
	return ""
}

// isoTime formats a sample's wall clock as ISO 8601 in UTC with an explicit
// offset, which sorts lexicographically in the same order as chronologically.
//
// The snapshot carries both clocks: the monotonic stamp is what the program
// compares, and the wall clock is only for putting the sample on a timeline a
// person or a spreadsheet can read. A zero wall clock yields an empty cell
// rather than a fabricated 1970 timestamp.
func isoTime(snapshot model.GpuSnapshot) string {
	if snapshot.WallClockNs <= 0 {
		return ""
	}
	return time.Unix(0, snapshot.WallClockNs).UTC().Format("2006-01-02T15:04:05.000+00:00")
}

// csvCell quotes a cell only when the value requires it.
func csvCell(value string) string {
	if !strings.ContainsAny(value, ",\"\n\r") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// sortStrings is an insertion sort: the lists here are at most a handful of
// throttle reasons, and this keeps a generic sort out of the exported API.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
