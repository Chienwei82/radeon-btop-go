package ui

import (
	"fmt"

	"github.com/gputop/gputop-go/internal/model"
)

// HeaderPanel builds the device header: name, driver, PCI id, uptime and
// interval.
func HeaderPanel(m model.AmdgpuMetrics, th Theme, uptimeS, intervalS float64) Row {
	device := m.Device
	var cells []Cell
	cells = append(cells, Cell{Text: device.Name, Color: th.Text + "|bold"})
	kind := "dGPU"
	if device.IsIntegrated() {
		kind = "iGPU"
	}
	cells = append(cells, Cell{Text: "  " + kind, Color: th.Accent})
	if device.BootVGA {
		cells = append(cells, Cell{Text: "  boot", Color: th.Warn})
	}
	cells = append(cells, Cell{Text: "\n", Color: ""})
	cells = append(cells, Cell{Text: "amdgpu", Color: th.Muted})
	cells = append(cells, Cell{Text: "  ·  ", Color: th.Track})
	cells = append(cells, Cell{Text: device.BDF, Color: th.Muted})
	cells = append(cells, Cell{Text: "  ·  ", Color: th.Track})
	cells = append(cells, Cell{Text: fmt.Sprintf("0x%04x", device.DeviceID), Color: th.Muted})
	if device.Render != nil {
		cells = append(cells, Cell{Text: "  ·  ", Color: th.Track})
		cells = append(cells, Cell{Text: *device.Render, Color: th.Muted})
	}
	cells = append(cells, Cell{Text: "\n", Color: ""})
	up := FmtDuration(&uptimeS)
	cells = append(cells, Cell{Text: padRight("up "+up, 24), Color: th.Muted})
	cells = append(cells, Cell{Text: fmt.Sprintf("every %gs", intervalS), Color: th.Muted})
	cells = append(cells, Cell{Text: "  ·  ", Color: th.Track})
	abi := "sysfs"
	abiColor := th.Warn
	if m.MetricsABI != nil {
		abi, abiColor = *m.MetricsABI, th.OK
	}
	cells = append(cells, Cell{Text: abi, Color: abiColor})
	return Row{Cells: cells}
}
