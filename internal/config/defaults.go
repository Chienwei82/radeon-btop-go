package config

import "github.com/gputop/gputop-go/internal/model"

// Default returns a Config with every value at its documented default. It is
// the base every load starts from and the result of loading a file that names
// nothing.
func Default() Config {
	return Config{
		IntervalMs:    1000,
		HistoryPoints: DefaultHistoryPoints,
		Gpu: GpuConfig{
			Kind:  "auto",
			Names: map[string]string{},
		},
		Process: ProcessConfig{
			Show:              true,
			MaxRows:           20,
			Sort:              string(model.SortTotal),
			MinUsagePercent:   0.0,
			HideKernelThreads: true,
		},
		UI: UiConfig{
			Theme: model.DefaultThemeName,
		},
		Blocks: BlocksConfig{
			Binary:    "radeontop",
			Ticks:     120,
			IntervalS: 1,
			DrmRoot:   "/dev/dri",
			Restart:   true,
		},
		Alerts: AlertsConfig{
			Enabled:      true,
			TempC:        90.0,
			PowerPercent: 95.0,
			VramPercent:  92.0,
			FlashHz:      1.0,
		},
		Log: LogConfig{
			ZstdLevel: 3,
			IntervalS: 0.0,
		},
	}
}
