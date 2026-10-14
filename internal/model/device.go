package model

import "fmt"

// DeviceKind classifies a GPU as integrated or discrete.
type DeviceKind string

const (
	DeviceIGPU DeviceKind = "igpu"
	DeviceDGPU DeviceKind = "dgpu"
)

// DeviceNames maps PCI device ids to marketing names for verified parts.
// Everything else renders as "AMD Radeon [0x....]".
var DeviceNames = map[int]string{
	0x73BF: "AMD Radeon RX 6800",
	0x6860: "AMD Vega 10 [Instinct MI25/MI25x2/V340/V320]",
	0x6861: "AMD Radeon PRO WX 9100",
	0x6863: "AMD Radeon Vega Frontier Edition",
	0x687F: "AMD Radeon RX Vega 56/64",
	0x1638: "AMD Radeon Graphics (Cezanne)",
	0x164E: "AMD Radeon Graphics (Raphael)",
}

// APUDeviceIDs are coarse device ids known to be integrated GPUs.
var APUDeviceIDs = map[int]struct{}{
	0x1550: {}, 0x1551: {}, 0x1552: {}, 0x1553: {}, 0x1554: {},
	0x1555: {}, 0x1556: {}, 0x1557: {}, 0x1558: {}, 0x1559: {},
	0x155A: {}, 0x155B: {}, 0x155C: {}, 0x155D: {}, 0x155E: {},
	0x155F: {}, 0x150C: {}, 0x150E: {}, 0x151C: {}, 0x1586: {},
	0x1638: {}, 0x163E: {}, 0x1640: {}, 0x164E: {}, 0x15BF: {},
	0x15C1: {},
}

// DescribeDeviceID returns a human readable name for a PCI device id.
func DescribeDeviceID(deviceID int) string {
	if name, ok := DeviceNames[deviceID]; ok {
		return name
	}
	return fmt.Sprintf("AMD Radeon [0x%04x]", deviceID)
}

// AmdgpuDevice is an immutable description of one amdgpu device.
type AmdgpuDevice struct {
	Index      int
	Card       string
	Render     *string
	BDF        Bdf
	VendorID   int
	DeviceID   int
	DeviceDir  string
	HwmonDir   *string
	Kind       DeviceKind
	Name       string
	VramVendor *string
	BootVGA    bool
}

// IsIntegrated reports whether this device is an APU/integrated GPU.
func (d AmdgpuDevice) IsIntegrated() bool { return d.Kind == DeviceIGPU }

// SupportsFan reports whether the panel can show a fan.
// Integrated GPUs never have a controllable fan.
func (d AmdgpuDevice) SupportsFan() bool { return !d.IsIntegrated() }

// Label returns a short label used in compact UI locations.
func (d AmdgpuDevice) Label() string { return d.Name + " (" + d.BDF + ")" }
