package ui

import (
	"github.com/gputop/gputop-go/internal/model"
)

func testMetrics() model.AmdgpuMetrics {
	render := "renderD128"
	return model.AmdgpuMetrics{
		Device: model.AmdgpuDevice{
			Index: 0, Card: "card0", Render: &render,
			BDF: "0000:0c:00.0", VendorID: 0x1002, DeviceID: 0x73BF,
			DeviceDir: "/sys/devices/fake", Kind: model.DeviceDGPU,
			Name: "AMD Radeon Test", BootVGA: false,
		},
		Sequence: 1, IntervalS: 1.0,
	}
}

func fullMetrics() model.AmdgpuMetrics {
	m := testMetrics()
	busy := model.Percent(50)
	cur := model.Mhz(2200)
	max := model.Mhz(2475)
	mcur := model.Mhz(1000)
	mmax := model.Mhz(1000)
	scur := model.Mhz(800)
	smax := model.Mhz(1200)
	m.GpuBusyPercent = &busy
	m.SCLK = model.Clock{Current: &cur, Maximum: &max}
	m.MCLK = model.Clock{Current: &mcur, Maximum: &mmax}
	m.SocClk = model.Clock{Current: &scur, Maximum: &smax}
	edge := model.Celsius(49)
	junction := model.Celsius(56)
	m.Temperatures = []model.Temperature{
		{Label: "edge", Celsius: &edge},
		{Label: "junction", Celsius: &junction},
	}
	draw, cap := model.Watts(45), model.Watts(211)
	m.Power = model.Power{DrawW: &draw, CapW: &cap}
	rpm, maxRPM, pwm := model.Rpm(2400), model.Rpm(3000), model.Percent(80)
	m.Fan = model.Fan{RPM: &rpm, MaxRPM: &maxRPM, PWM: &pwm}
	m.VoltagesMv = []model.VoltageReading{{Name: "soc", MV: 918}, {Name: "gfx", MV: 768}}
	m.PCIE = &model.PcieLink{Width: 16, Speed: 80}
	prof := "BOOTUP_DEFAULT"
	m.PowerProfile = &prof
	m.Throttle = &model.ThrottleInfo{Raw: 0}
	return m
}
