package config

import "strings"

// applyDocument overlays the decoded document onto a default Config.
//
// Each section is built by reading only the keys it knows about, so an unknown
// key inside a section is skipped silently here and reported separately by the
// caller's unknown-key scan. Every value passes through its as* coercion, so a
// wrong type becomes a warning and a default rather than a panic.
func applyDocument(c *Config, raw map[string]any, warnings *[]string) {
	if v, ok := raw["interval_ms"]; ok {
		c.IntervalMs = asInt(v, c.IntervalMs, "interval_ms", warnings)
	}
	if v, ok := raw["history_points"]; ok {
		c.HistoryPoints = asInt(v, c.HistoryPoints, "history_points", warnings)
	}
	if section := asSection(raw["gpu"]); section != nil {
		applyGpu(&c.Gpu, section, warnings)
	}
	if section := asSection(raw["process"]); section != nil {
		applyProcess(&c.Process, section, warnings)
	}
	if section := asSection(raw["ui"]); section != nil {
		applyUI(&c.UI, section, warnings)
	}
	if section := asSection(raw["blocks"]); section != nil {
		applyBlocks(&c.Blocks, section, warnings)
	}
	if section := asSection(raw["alerts"]); section != nil {
		applyAlerts(&c.Alerts, section, warnings)
	}
	if section := asSection(raw["log"]); section != nil {
		applyLog(&c.Log, section, warnings)
	}
	if section := asSection(raw["state"]); section != nil {
		applyStateSection(&c.State, section, warnings)
	}
}

func applyGpu(g *GpuConfig, section map[string]any, warnings *[]string) {
	if v, ok := section["devices"]; ok {
		g.Devices = asStrList(v, "gpu.devices", warnings)
	}
	if v, ok := section["kind"]; ok {
		g.Kind = asStr(v, g.Kind, "gpu.kind", warnings)
	}
	if v, ok := section["names"]; ok {
		g.Names = asStrMap(v, "gpu.names", warnings)
	}
}

func applyProcess(p *ProcessConfig, section map[string]any, warnings *[]string) {
	if v, ok := section["show"]; ok {
		p.Show = asBool(v, p.Show, "process.show", warnings)
	}
	if v, ok := section["max_rows"]; ok {
		p.MaxRows = asInt(v, p.MaxRows, "process.max_rows", warnings)
	}
	if v, ok := section["sort"]; ok {
		p.Sort = asStr(v, p.Sort, "process.sort", warnings)
	}
	if v, ok := section["min_usage_percent"]; ok {
		p.MinUsagePercent = asFloat(v, p.MinUsagePercent, "process.min_usage_percent", warnings)
	}
	if v, ok := section["hide_kernel_threads"]; ok {
		p.HideKernelThreads = asBool(v, p.HideKernelThreads, "process.hide_kernel_threads", warnings)
	}
	if v, ok := section["process_tree"]; ok {
		p.ProcessTree = asBool(v, p.ProcessTree, "process.process_tree", warnings)
	}
	if v, ok := section["full_command"]; ok {
		p.FullCommand = asBool(v, p.FullCommand, "process.full_command", warnings)
	}
	if v, ok := section["show_idle"]; ok {
		p.ShowIdle = asBool(v, p.ShowIdle, "process.show_idle", warnings)
	}
	if v, ok := section["allow_kill"]; ok {
		p.AllowKill = asBool(v, p.AllowKill, "process.allow_kill", warnings)
	}
}

func applyUI(u *UiConfig, section map[string]any, warnings *[]string) {
	if v, ok := section["theme"]; ok {
		u.Theme = asStr(v, u.Theme, "ui.theme", warnings)
	}
	if v, ok := section["no_color"]; ok {
		u.NoColor = asBool(v, u.NoColor, "ui.no_color", warnings)
	}
	if v, ok := section["graph_height"]; ok {
		u.GraphHeight = asInt(v, u.GraphHeight, "ui.graph_height", warnings)
	}
}

func applyBlocks(b *BlocksConfig, section map[string]any, warnings *[]string) {
	if v, ok := section["enabled"]; ok {
		b.Enabled = asBool(v, b.Enabled, "blocks.enabled", warnings)
	}
	if v, ok := section["binary"]; ok {
		b.Binary = asStr(v, b.Binary, "blocks.binary", warnings)
	}
	if v, ok := section["ticks"]; ok {
		b.Ticks = asInt(v, b.Ticks, "blocks.ticks", warnings)
	}
	if v, ok := section["interval_s"]; ok {
		b.IntervalS = asInt(v, b.IntervalS, "blocks.interval_s", warnings)
	}
	if v, ok := section["drm_root"]; ok {
		b.DrmRoot = asStr(v, b.DrmRoot, "blocks.drm_root", warnings)
	}
	if v, ok := section["restart"]; ok {
		b.Restart = asBool(v, b.Restart, "blocks.restart", warnings)
	}
}

func applyAlerts(a *AlertsConfig, section map[string]any, warnings *[]string) {
	if v, ok := section["enabled"]; ok {
		a.Enabled = asBool(v, a.Enabled, "alerts.enabled", warnings)
	}
	if v, ok := section["temp_c"]; ok {
		a.TempC = asFloat(v, a.TempC, "alerts.temp_c", warnings)
	}
	if v, ok := section["power_percent"]; ok {
		a.PowerPercent = asFloat(v, a.PowerPercent, "alerts.power_percent", warnings)
	}
	if v, ok := section["vram_percent"]; ok {
		a.VramPercent = asFloat(v, a.VramPercent, "alerts.vram_percent", warnings)
	}
	if v, ok := section["flash_hz"]; ok {
		a.FlashHz = asFloat(v, a.FlashHz, "alerts.flash_hz", warnings)
	}
}

func applyLog(l *LogConfig, section map[string]any, warnings *[]string) {
	if v, ok := section["zstd_level"]; ok {
		l.ZstdLevel = asInt(v, l.ZstdLevel, "log.zstd_level", warnings)
	}
	if v, ok := section["interval_s"]; ok {
		l.IntervalS = asFloat(v, l.IntervalS, "log.interval_s", warnings)
	}
}

// applyStateSection builds the state block from its file keys, which differ
// from the struct field names (the file says process_sort and process_filter
// where the struct says sort and filter) so the section reads well beside
// [process].
func applyStateSection(s *StateConfig, section map[string]any, warnings *[]string) {
	if v, ok := section["theme"]; ok {
		s.Theme = optionalStr(v, "state.theme", warnings)
	}
	if v, ok := section["interval_ms"]; ok {
		s.IntervalMs = optionalInt(v, "state.interval_ms", warnings)
	}
	if v, ok := section["process_sort"]; ok {
		s.Sort = optionalStr(v, "state.process_sort", warnings)
	}
	if v, ok := section["process_sort_reverse"]; ok {
		s.SortReverse = optionalBool(v, "state.process_sort_reverse", warnings)
	}
	if v, ok := section["process_filter"]; ok {
		s.Filter = asStr(v, s.Filter, "state.process_filter", warnings)
	}
	if v, ok := section["show_idle_processes"]; ok {
		s.ShowIdle = asBool(v, s.ShowIdle, "state.show_idle_processes", warnings)
	}
	if v, ok := section["show_process_tree"]; ok {
		s.ProcessTree = asBool(v, s.ProcessTree, "state.show_process_tree", warnings)
	}
	if v, ok := section["full_command"]; ok {
		s.FullCommand = asBool(v, s.FullCommand, "state.full_command", warnings)
	}
	if v, ok := section["overview"]; ok {
		s.Overview = asBool(v, s.Overview, "state.overview", warnings)
	}
	if v, ok := section["gpu_index"]; ok {
		s.GpuIndex = asInt(v, s.GpuIndex, "state.gpu_index", warnings)
	}
}

// optionalStr returns a pointer to a string value, or nil when the value is
// null. A nil means "not recorded", which must stay distinct from a recorded
// empty string.
func optionalStr(value any, where string, warnings *[]string) *string {
	if value == nil {
		return nil
	}
	s := asStr(value, "", where, warnings)
	return &s
}

// optionalInt returns a pointer to an int value, or nil when the value is
// null.
func optionalInt(value any, where string, warnings *[]string) *int {
	if value == nil {
		return nil
	}
	n := asInt(value, 0, where, warnings)
	return &n
}

// optionalBool returns a pointer to a bool value, or nil when the value is
// null.
func optionalBool(value any, where string, warnings *[]string) *bool {
	if value == nil {
		return nil
	}
	b := asBool(value, false, where, warnings)
	return &b
}

// trimSpace trims surrounding whitespace, the Go counterpart of str.strip used
// on list entries.
func trimSpace(s string) string { return strings.TrimSpace(s) }
