package check

import "runtime"

// ToMap is the JSON form of the report: the same facts the text report
// carries, in a shape a script can read.
//
// The environment block substitutes "go" for the Python build's
// "python"/"free_threaded" pair: a Go binary's interpreter facts are the
// toolchain that built it, and whether it is a static binary. The
// substitution is deliberate -- this is a reimplementation, so it owes the
// same answers, not the same keys.
func (r Report) ToMap() map[string]any {
	devices := make([]any, 0, len(r.Devices))
	for _, device := range r.Devices {
		findings := make([]any, 0, len(device.Findings))
		for _, finding := range device.Findings {
			findings = append(findings, findingMap(finding))
		}
		devices = append(devices, map[string]any{
			"index":       device.Device.Index,
			"card":        device.Device.Card,
			"bdf":         device.Device.BDF,
			"name":        device.Device.Name,
			"kind":        string(device.Device.Kind),
			"device_id":   hexDeviceID(device.Device.DeviceID),
			"metrics_abi": nullableString(device.MetricsABI),
			"available":   len(device.Findings) - len(device.Missing()),
			"metrics":     findings,
		})
	}
	system := make([]any, 0, len(r.System))
	for _, finding := range r.System {
		system = append(system, findingMap(finding))
	}
	warnings := make([]any, 0, len(r.Warnings))
	for _, warning := range r.Warnings {
		warnings = append(warnings, warning)
	}
	renderNodes := make([]string, 0, len(r.RenderNodes))
	renderNodes = append(renderNodes, r.RenderNodes...)
	return map[string]any{
		"ok": r.OK(),
		"environment": map[string]any{
			"kernel":       r.Kernel,
			"amdgpu":       nullableString(r.Amdgpu),
			"go":           runtime.Version(),
			"sysfs":        r.DrmRoot,
			"procfs":       r.ProcRoot,
			"render_nodes": renderNodes,
		},
		"devices":  devices,
		"system":   system,
		"warnings": warnings,
	}
}

// findingMap is one finding's JSON form.
func findingMap(f Finding) map[string]any {
	reading := any(nil)
	if f.Metric.Value != nil {
		reading = *f.Metric.Value
	}
	reason := any(nil)
	if f.Reason != "" {
		reason = string(f.Reason)
	}
	return map[string]any{
		"metric":  f.Metric.Key,
		"label":   f.Metric.Label,
		"status":  string(f.Availability),
		"source":  f.Metric.Source,
		"reading": reading,
		"reason":  reason,
		"detail":  f.Detail,
		"remedy":  f.Metric.Remedy,
	}
}

// nullableString keeps "unknown" out of a JSON string field: an empty version
// is null, not "".
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// hexDeviceID is the marketing id in the reference's 0x form.
func hexDeviceID(id int) string {
	const digits = "0123456789abcdef"
	out := []byte{'0', 'x', '0', '0', '0', '0'}
	value := id
	for i := 5; i >= 2; i-- {
		out[i] = digits[value&0xf]
		value >>= 4
	}
	return string(out)
}
