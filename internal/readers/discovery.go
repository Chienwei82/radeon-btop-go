package readers

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// CardPattern matches card0, card12 but never connector nodes.
var CardPattern = regexp.MustCompile(`^card\d+$`)

// BDFPattern matches a PCI domain:bus:slot.function address.
var BDFPattern = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$`)

// AMDVendorID is the PCI vendor id for AMD.
const AMDVendorID = 0x1002

// IGPUVRAMCeiling: APU carve-outs are small; above this is discrete.
const IGPUVRAMCeiling = 512 * 1024 * 1024

// GTTToVRAMRatio: GTT much larger than VRAM implies integration.
const GTTToVRAMRatio = 4

func bdfFromDeviceDir(deviceDir string) string {
	resolved, ok := Resolve(deviceDir)
	if !ok {
		resolved = deviceDir
	}
	name := filepath.Base(resolved)
	if BDFPattern.MatchString(name) {
		return name
	}
	return ""
}

func isAmdgpu(deviceDir string) bool {
	target, ok := LinkTarget(filepath.Join(deviceDir, "driver"))
	if !ok {
		return false
	}
	parts := strings.Split(target, "/")
	return parts[len(parts)-1] == "amdgpu"
}

func resolveHwmon(deviceDir string) string {
	hwmonLink := filepath.Join(deviceDir, "hwmon")
	entries, err := os.ReadDir(hwmonLink)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "hwmon") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return filepath.Join(hwmonLink, names[0])
}

var renderPattern = regexp.MustCompile(`^renderD\d+$`)

func findRenderNode(deviceDir string) string {
	entries, err := os.ReadDir(filepath.Join(deviceDir, "drm"))
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, n := range names {
		if renderPattern.MatchString(n) {
			return n
		}
	}
	return ""
}

func classify(deviceDir string, deviceID int, vramVendor *string, forced *model.DeviceKind) model.DeviceKind {
	if forced != nil {
		return *forced
	}
	votes := 0
	if _, ok := model.APUDeviceIDs[deviceID]; ok {
		votes++
	}
	if vramVendor != nil && strings.TrimSpace(strings.ToUpper(*vramVendor)) == "N/A" {
		votes++
	}
	if total, ok := ReadInt(filepath.Join(deviceDir, "mem_info_vram_total")); ok {
		if total <= IGPUVRAMCeiling {
			votes++
		}
	}
	if vram, ok := ReadInt(filepath.Join(deviceDir, "mem_info_vram_total")); ok && vram > 0 {
		if gtt, ok := ReadInt(filepath.Join(deviceDir, "mem_info_gtt_total")); ok {
			if gtt >= vram*GTTToVRAMRatio {
				votes++
			}
		}
	}
	if votes >= 2 {
		return model.DeviceIGPU
	}
	return model.DeviceDGPU
}

// DeviceTokens returns every name a device answers to for filtering.
func DeviceTokens(d model.AmdgpuDevice) map[string]bool {
	out := map[string]bool{
		strings.ToLower(d.BDF):  true,
		strings.ToLower(d.Card): true,
	}
	if len(d.BDF) > 5 {
		out[strings.ToLower(d.BDF[5:])] = true
	}
	return out
}

// SelectDevices filters devices by requested names, warning on misses.
func SelectDevices(devices []model.AmdgpuDevice, requested []string) ([]model.AmdgpuDevice, []string) {
	wanted := map[string]bool{}
	var order []string
	for _, name := range requested {
		n := strings.TrimSpace(strings.ToLower(name))
		if n != "" && !wanted[n] {
			wanted[n] = true
			order = append(order, n)
		}
	}
	if len(wanted) == 0 {
		return devices, nil
	}
	var selected []model.AmdgpuDevice
	for _, d := range devices {
		tokens := DeviceTokens(d)
		for w := range wanted {
			if tokens[w] {
				selected = append(selected, d)
				break
			}
		}
	}
	if len(selected) > 0 {
		var missing []string
		for _, w := range order {
			found := false
			for _, d := range devices {
				if DeviceTokens(d)[w] {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, w)
			}
		}
		sort.Strings(missing)
		return selected, missing
	}
	missed := append([]string{}, order...)
	sort.Strings(missed)
	return devices, missed
}

// DiscoverDevices enumerates every amdgpu GPU under drmRoot.
func DiscoverDevices(drmRoot string, kindOverride *model.DeviceKind, nameOverrides map[string]string, pciRoot string) []model.AmdgpuDevice {
	if drmRoot == "" {
		drmRoot = "/sys/class/drm"
	}
	if pciRoot == "" {
		pciRoot = "/sys/bus/pci/devices"
	}
	entries, err := os.ReadDir(drmRoot)
	if err != nil {
		return nil
	}
	type found struct {
		igpu   bool
		bdf    string
		device model.AmdgpuDevice
	}
	var list []found
	for _, entry := range entries {
		if !CardPattern.MatchString(entry.Name()) {
			continue
		}
		deviceDir := filepath.Join(drmRoot, entry.Name(), "device")
		vendorID, ok := ReadHexInt(filepath.Join(deviceDir, "vendor"))
		if !ok || vendorID != AMDVendorID {
			continue
		}
		if !isAmdgpu(deviceDir) {
			continue
		}
		deviceID, ok := ReadHexInt(filepath.Join(deviceDir, "device"))
		if !ok {
			continue
		}
		bdf := bdfFromDeviceDir(deviceDir)
		var vramVendor *string
		if text, ok := ReadText(filepath.Join(deviceDir, "mem_info_vram_vendor"), ValueLimit); ok {
			vramVendor = &text
		}
		kind := classify(deviceDir, deviceID, vramVendor, kindOverride)
		var bootVGA bool
		if bdf != "" {
			if text, ok := ReadText(filepath.Join(pciRoot, bdf, "boot_vga"), ValueLimit); ok && text == "1" {
				bootVGA = true
			}
		}
		resolved, ok := Resolve(deviceDir)
		if !ok {
			resolved = deviceDir
		}
		var hwmonDir *string
		if h := resolveHwmon(deviceDir); h != "" {
			if r, ok := Resolve(h); ok {
				hwmonDir = &r
			} else {
				hwmonDir = &h
			}
		}
		render := findRenderNode(deviceDir)
		var renderPtr *string
		if render != "" {
			renderPtr = &render
		}
		name := model.DescribeDeviceID(deviceID)
		if nameOverrides != nil {
			if o, ok := nameOverrides[bdf]; ok && o != "" {
				name = o
			}
		}
		list = append(list, found{kind == model.DeviceIGPU, bdf, model.AmdgpuDevice{
			Card: entry.Name(), Render: renderPtr, BDF: bdf,
			VendorID: vendorID, DeviceID: deviceID, DeviceDir: resolved,
			HwmonDir: hwmonDir, Kind: kind, Name: name,
			VramVendor: vramVendor, BootVGA: bootVGA,
		}})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].igpu != list[j].igpu {
			return !list[i].igpu
		}
		return list[i].bdf < list[j].bdf
	})
	out := make([]model.AmdgpuDevice, 0, len(list))
	for i, item := range list {
		item.device.Index = i
		out = append(out, item.device)
	}
	return out
}
