package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/config"
	"github.com/gputop/gputop-go/internal/sampler"
)

// checkOneShot points --check at an empty DRM class directory, so the report
// is about the machine it runs on without needing a GPU.
func checkOneShot() OneShot {
	cfg := config.Default()
	return OneShot{
		Config: &cfg,
		Sampler: sampler.SamplerOptions{
			DrmRoot: "/nonexistent/drm", ProcRoot: "/proc", CollectProcesses: true,
		},
	}
}

func TestCheckWithoutDevicesExplainsAndExitsNoDevice(t *testing.T) {
	var out bytes.Buffer
	status, err := Check(&out, checkOneShot(), CheckOptions{
		ProcRoot: "/proc", ProcessesScanned: true, BlocksBinary: "radeontop",
	})
	if err != nil {
		t.Fatalf("Check = %v, want nil", err)
	}
	if status != ExitNoDevice {
		t.Fatalf("status = %d, want %d", status, ExitNoDevice)
	}
	text := out.String()
	for _, want := range []string{
		"gputop check", "No amdgpu device was found", "Process table", "Per-block panel",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report missing %q:\n%s", want, text)
		}
	}
}

func TestCheckJSONHasTheScriptableShape(t *testing.T) {
	var out bytes.Buffer
	if _, err := Check(&out, checkOneShot(), CheckOptions{
		JSON: true, ProcRoot: "/proc", ProcessesScanned: true, BlocksBinary: "radeontop",
	}); err != nil {
		t.Fatalf("Check = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("--check --json is not valid JSON: %v\n%s", err, out.String())
	}
	for _, key := range []string{"ok", "environment", "devices", "system", "warnings"} {
		if _, present := payload[key]; !present {
			t.Errorf("missing top-level key %q", key)
		}
	}
	environment, ok := payload["environment"].(map[string]any)
	if !ok {
		t.Fatalf("environment = %#v", payload["environment"])
	}
	for _, key := range []string{"kernel", "amdgpu", "go"} {
		if _, present := environment[key]; !present {
			t.Errorf("environment is missing %q", key)
		}
	}
	system, ok := payload["system"].([]any)
	if !ok || len(system) != 2 {
		t.Fatalf("system = %#v, want the two machine findings", payload["system"])
	}
	keys := map[string]bool{}
	for _, entry := range system {
		finding, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("system entry = %#v", entry)
		}
		keys[finding["metric"].(string)] = true
	}
	if !keys["processes"] || !keys["blocks"] {
		t.Errorf("system metrics = %v, want processes and blocks", keys)
	}
}

func TestCheckReportsTheProcScanBeingSwitchedOff(t *testing.T) {
	var out bytes.Buffer
	if _, err := Check(&out, checkOneShot(), CheckOptions{
		ProcRoot: "/proc", ProcessesScanned: false, BlocksBinary: "radeontop",
	}); err != nil {
		t.Fatalf("Check = %v", err)
	}
	if !strings.Contains(out.String(), "switched off") {
		t.Fatalf("report does not name the switched-off scan:\n%s", out.String())
	}
}
