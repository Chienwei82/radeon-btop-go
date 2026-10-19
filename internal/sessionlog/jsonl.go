package sessionlog

import (
	"encoding/json"

	"github.com/gputop/gputop-go/internal/model"
)

// jsonDevice is the device identity block of one JSON Lines record.
type jsonDevice struct {
	Index      int    `json:"index"`
	Card       string `json:"card"`
	BDF        string `json:"bdf"`
	Name       string `json:"name"`
	Integrated bool   `json:"integrated"`
}

// jsonPool is a memory pool. nil members are written as null, never 0.
type jsonPool struct {
	Used    *model.Bytes   `json:"used"`
	Total   *model.Bytes   `json:"total"`
	Percent *model.Percent `json:"percent"`
}

// jsonClocks is the three clock frequencies in MHz.
type jsonClocks struct {
	SCLK   *model.Mhz `json:"sclk"`
	MCLK   *model.Mhz `json:"mclk"`
	SocClk *model.Mhz `json:"socclk"`
}

// jsonPower is draw, cap and their ratio.
type jsonPower struct {
	DrawW   *model.Watts   `json:"draw_w"`
	CapW    *model.Watts   `json:"cap_w"`
	Percent *model.Percent `json:"percent"`
}

// jsonBlocks is the radeontop-derived per-block utilisation.
type jsonBlocks struct {
	Busiest string                   `json:"busiest"`
	Percent map[string]model.Percent `json:"percent"`
}

// jsonSample is one device at one instant: the JSON Lines equivalent of a CSV
// row.
type jsonSample struct {
	Time        string                    `json:"time"`
	MonotonicNs model.Nanoseconds         `json:"monotonic_ns"`
	Sequence    int                       `json:"sequence"`
	Device      jsonDevice                `json:"device"`
	GpuPercent  *model.Percent            `json:"gpu_percent"`
	MemPercent  *model.Percent            `json:"mem_percent"`
	VRAM        jsonPool                  `json:"vram"`
	GTT         jsonPool                  `json:"gtt"`
	Clocks      jsonClocks                `json:"clocks_mhz"`
	Power       jsonPower                 `json:"power"`
	FanRPM      *model.Rpm                `json:"fan_rpm"`
	Temps       map[string]*model.Celsius `json:"temperatures_c"`
	Profile     *string                   `json:"profile"`
	Throttling  []string                  `json:"throttling"`
	ODC         string                    `json:"odc"`
	Blocks      *jsonBlocks               `json:"blocks"`
}

// jsonRecord builds one JSON object for one device's sample. An absent
// reading is null, never 0 and never a string: the JSON equivalent of the
// CSV's empty cell.
func jsonRecord(snapshot model.GpuSnapshot, metrics *model.AmdgpuMetrics) (string, error) {
	temps := make(map[string]*model.Celsius, len(metrics.Temperatures))
	for _, t := range metrics.Temperatures {
		temps[t.Label] = t.Celsius
	}

	var profile *string
	if value := profileName(metrics); value != "" {
		profile = &value
	}
	var throttling []string
	if metrics.Throttle != nil {
		throttling = append(throttling, metrics.Throttle.Active...)
		sortStrings(throttling)
	}
	var blocks *jsonBlocks
	if metrics.Blocks != nil {
		blocks = &jsonBlocks{Busiest: metrics.Blocks.BusiestKey(), Percent: metrics.Blocks.ByKey()}
	}

	sample := jsonSample{
		Time:        isoTime(snapshot),
		MonotonicNs: snapshot.TimestampNs,
		Sequence:    snapshot.Sequence,
		Device: jsonDevice{
			Index:      metrics.Device.Index,
			Card:       metrics.Device.Card,
			BDF:        metrics.Device.BDF,
			Name:       metrics.Device.Name,
			Integrated: metrics.Device.IsIntegrated(),
		},
		GpuPercent: metrics.GpuBusyPercent,
		MemPercent: metrics.MemBusyPercent,
		VRAM:       jsonPool{Used: metrics.VRAM.Used, Total: metrics.VRAM.Total, Percent: metrics.VRAM.Percent()},
		GTT:        jsonPool{Used: metrics.GTT.Used, Total: metrics.GTT.Total, Percent: metrics.GTT.Percent()},
		Clocks:     jsonClocks{SCLK: metrics.SCLK.Current, MCLK: metrics.MCLK.Current, SocClk: metrics.SocClk.Current},
		Power:      jsonPower{DrawW: metrics.Power.DrawW, CapW: metrics.Power.CapW, Percent: metrics.Power.Percent()},
		FanRPM:     metrics.Fan.RPM,
		Temps:      temps,
		Profile:    profile,
		Throttling: throttling,
		ODC:        metrics.ODC.Describe(),
		Blocks:     blocks,
	}
	payload, err := json.Marshal(sample)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
