package readers

// Throttle bit layout mirrors drivers/gpu/drm/amd/pm/inc/amdgpu_smu.h.
var throttleBits = [][2]int{
	{1 << 0, 0}, {1 << 1, 1}, {1 << 2, 2}, {1 << 3, 3},
	{1 << 4, 4}, {1 << 5, 5}, {1 << 6, 6}, {1 << 7, 7},
}

var throttleNames = map[int]string{
	1 << 0:  "PPT0",
	1 << 1:  "PPT1",
	1 << 2:  "PPT2",
	1 << 3:  "PPT3",
	1 << 4:  "SPL",
	1 << 5:  "FPPT",
	1 << 6:  "SPPT",
	1 << 7:  "SPPT_APU",
	1 << 16: "TDC_GFX",
	1 << 17: "TDC_SOC",
	1 << 18: "TDC_MEM",
	1 << 19: "TDC_VDD",
	1 << 20: "TDC_CVIP",
	1 << 21: "EDC_CPU",
	1 << 22: "EDC_GFX",
	1 << 23: "APCC",
	1 << 32: "TEMP_GPU",
	1 << 33: "TEMP_CORE",
	1 << 34: "TEMP_MEM",
	1 << 35: "TEMP_EDGE",
	1 << 36: "TEMP_HOTSPOT",
	1 << 37: "TEMP_SOC",
	1 << 38: "TEMP_VR_GFX",
	1 << 39: "TEMP_VR_SOC",
	1 << 40: "TEMP_VR_MEM0",
	1 << 41: "TEMP_VR_MEM1",
	1 << 42: "TEMP_LIQUID0",
	1 << 43: "TEMP_LIQUID1",
	1 << 44: "VRHOT0",
	1 << 45: "VRHOT1",
	1 << 46: "PROCHOT_CPU",
	1 << 47: "PROCHOT_GFX",
	1 << 56: "PPM",
	1 << 57: "FIT",
}

// DecodeThrottle returns the names of every throttle bit set in raw.
func DecodeThrottle(raw int) []string {
	if raw <= 0 {
		return nil
	}
	var out []string
	for bit, name := range throttleNames {
		if raw&bit != 0 {
			out = append(out, name)
		}
	}
	return out
}

var _ = throttleBits
