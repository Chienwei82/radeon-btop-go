// Package model is the domain model of gputop.
//
// Every type here is pure data: no I/O, no TUI, no logging. Optional
// metrics are pointers: nil means "not available on this hardware /
// kernel / permission level" and must be rendered as N/A by the UI,
// never as zero.
package model

// Unit aliases shared across the data layer.
type (
	// Mhz is a clock frequency in megahertz.
	Mhz = int
	// Bytes is a byte count.
	Bytes = int64
	// Rpm is a fan speed in revolutions per minute.
	Rpm = int
	// Celsius is a temperature in degrees Celsius.
	Celsius = float64
	// Watts is a power figure in watts.
	Watts = float64
	// Percent is a 0-100 percentage.
	Percent = float64
	// Bdf is a PCI address in domain:bus:device.function form.
	Bdf = string
	// ClientId is a DRM client identifier.
	ClientId = int
	// Nanoseconds is a duration or timestamp in nanoseconds.
	Nanoseconds = int64
)
