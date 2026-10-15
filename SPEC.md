# gputop-go — Specification

**Working name:** `gputop` (module `github.com/gputop/gputop-go`, binary `gputop`)
**Status:** Living specification of the Go port. The Python reference
(`/home/david/Codigo/radeon-btop-py/`, SPEC.md there) is ground truth for
behavior; this file adapts it to Go and records every deliberate deviation.
**Reference hardware:** AMD Radeon Navi 21 / RX 6800 (`0x73bf`, RDNA2),
`amdgpu` 6.19.4, kernel `7.0.0-34-generic`.

---

## 1. Purpose and goals

`gputop` is a btop-style terminal UI for monitoring **AMD Radeon GPUs driven
by the `amdgpu` kernel driver** — a better replacement for `radeontop`,
delivered as one static Go binary.

### 1.1 Goals

| # | Goal | Measurable criterion |
|---|------|----------------------|
| G1 | **Comprehensive** metrics in one screen | VRAM, GTT, busy %, clocks, 3 temperatures, power vs cap, fan, perf level, power profile, throttle status, per-process engine usage — all on one screen |
| G2 | **Per-process visibility** | Per-PID VRAM/GTT and per-engine utilisation, deduplicated by `drm-client-id` |
| G3 | **Unprivileged** operation | **Every core metric** works as an ordinary user; per-process table is partial without root — stated in the UI |
| G4 | **Robustness** | Missing files, permissions, kernel drift, truncated binaries degrade to `N/A`; never crashes, never hangs |
| G5 | **Multi-GPU** | Any number of `amdgpu` devices, dGPU + iGPU, runtime switching |
| G6 | **APU-aware** | iGPU detected → memory panel leads with GTT |
| G7 | **Read-only** | Never writes to sysfs (AST scan of `internal/readers`, enforced in CI) |
| G8 | **Single static binary** | `CGO_ENABLED=0`; distroless release; no interpreter on user machines |

### 1.2 Non-goals

- **No NVIDIA or Intel support.** No vendor abstraction layer.
- **No AMDGPU/ADLX/ROCm SMI library.**
- **No GPU control** in v1. Profile/overdrive attributes are read, never written.
- **No cross-platform support.** Linux only.
- **No implicit persistence.** History is an in-memory ring; gone on exit.
  Only `--log PATH` and the `[state]` section write.

### 1.3 What "better than radeontop" means concretely

1. Engine-level per-process detail (gfx/compute/DMA/enc/dec).
2. Many GPUs at once, not just one.
3. btop-style presentation: braille sparklines, dual memory bars.
4. Clean `N/A` semantics (`nil`, never `0`).
5. Modern delivery: one binary, podman-built, `go test` suite.

---

## 2. Platform and requirements

| Item | Requirement |
|------|-------------|
| OS | Linux, `CONFIG_DRM_AMDGPU=m` or built-in |
| **Minimum kernel** | **5.19** (DRM fdinfo accounting keys) |
| Architecture | any; no arch-specific code; C-struct padding reproduced manually, never host-native alignment |
| Go | **1.27.x**, pinned in `Containerfile`; toolchain runs ONLY in podman |
| UI | bubbletea v2 + lipgloss + bubbles |
| Go deps | bubbletea, lipgloss, bubbles, goccy/go-yaml, klauspost/compress. No libdrm bindings, no cgo |
| Optional external binary | `radeontop` on `$PATH`, only for the optional per-block panel |

### 2.1 Non-negotiable engineering constraints

- **Immutability across goroutines**: sampler and UI exchange **immutable
  snapshots** through a bounded `chan` (capacity 1–2; stale dropped). No
  shared mutable state.
- **YAML config**: goccy/go-yaml for reading (`gputop.example.yaml`; the
  Python build's TOML is *not* the wire format — this port only owes
  functional parity, not file compatibility). Unknown keys are found by
  decoding the same document into a generic map and diffing it against the
  known key set, which feeds warnings instead of errors. `state:` write-back
  round-trips the parsed AST (`parser.ParseComments` +
  `Path.ReplaceWithNode`), so comments and key order survive; atomic via
  tmp+fsync+rename.
- **Read-only**: a CI job parses every non-test file in `internal/readers` and
  fails on a filesystem mutator, a write-capable open (`os.OpenFile`/`syscall.Open`
  with write flags, or flags it cannot resolve), a `Write*`/`Truncate`/`Chmod`
  method, a `fmt.Fprint*` aimed anywhere but a standard stream, or a shell
  redirect; process launchers are confined to `radeontop.go`. The scan works on
  the AST, so prose that merely mentions a write does not trip it. A test also
  asserts the SCPP/overdrive tables are byte-identical after reading.
- **Gates**: `gofmt` + `go vet` + `golangci-lint` + `go test ./...` green
  inside the container before every commit.

---

## 3. Metrics specification

Every metric is optional. `nil` renders as `N/A` in the UI and as an empty
cell in CSV. "Never" means the value is not available on this hardware/kernel.

### 3.1 Core metrics

Same set as the Python reference: `gpu_busy_percent`, `mem_busy_percent`,
`vram_used/total`, `gtt_used/total`, `sclk/mclk (+max)`, `temp_edge`,
`temp_junction`, `temp_mem`, `power_draw`, `power_cap`, `fan_rpm`,
`fan_percent` (RPM / `fan1_max`), `performance_level`, `power_profile` (+full
table), `odv_<domain>` (unit carried per entry), `vddgfx_offset`,
`blocks.<key>` (optional radeontop child), `throttle_status`, `pcie_link`
(speed in **0.1 GT/s**, never a generation; generation derived),
`voltage_soc/gfx/mem` (`gpu_metrics` only; 0/sentinel = `N/A`).

### 3.2 Per-process metrics

From `/proc/<pid>/fdinfo/<fd>` of DRM/render-node fds: cumulative
`drm-engine-<name>` ns (diffed across ticks), `drm-*-vram/gtt` (IEC `KiB`),
`drm-client-id`, `drm-pdev`, `drm-driver`, `amd-requested-vram/gtt`,
`amd-evicted-vram`. Identity from `/proc/<pid>/{comm,status}` (`PPid` for
tree, full `cmdline` on demand).

**Deduplication:** same `drm-client-id` on the same `drm-pdev` = one client;
merge takes per-field **maximums**, never sums. Mesa helper children with
their own client-ids are separate rows.

### 3.3 Source precedence (highest first)

1. `gpu_metrics` — temps, instantaneous power, throttle flags.
2. Device sysfs — VRAM/GTT totals/used, busy %, link.
3. hwmon — temperatures, power average/cap, fan, pwm.
4. DPM tables — clocks; **clock maxima ALWAYS from sysfs/DPM, never binary**.
5. `/proc/*/fdinfo` — per-process.

### 3.4 Optional metrics (absent unless asked)

**Per-block utilisation** via supervised `radeontop` child (off by default);
**overdrive** `pp_od_clk_voltage` in both eras (legacy voltage table vs
sectioned clock table; unit per entry; bar ratio against the driver's own
`OD_RANGE`). Neither attribute is ever written.

---

## 4. Functional requirements

| ID | Requirement |
|----|-------------|
| FR1 | Sampling runs in a **background goroutine**, never blocks the UI; enforces the configured interval (default **1000 ms**) on a monotonic deadline grid; slow ticks drop missed slots, keep phase |
| FR2 | The UI reads only the **latest** snapshot from a bounded channel (cap 1–2); stale snapshots are discarded, never queued |
| FR3 | Configurable interval via YAML config and CLI flag |
| FR4 | Per-process table sorted by a user-selectable column; default = total engine usage |
| FR5 | GPU switching without restarting, by key and by picker overlay |
| FR6 | APU awareness: iGPU detected → memory panel leads with GTT |
| FR7 | Any missing/inaccessible metric renders `N/A` and never panics |
| FR8 | Headless/diagnostic modes print a text/JSON report and exit |
| FR9 | Graphs show a rolling window of history with the configured retention |
| FR10 | Read-only: no sysfs write path exists in the codebase |
| FR11 | **Optional** per-block panel, off by default, from a supervised `radeontop` child; hides itself and states the reason |
| FR12 | **Optional threshold alerts** for temperature, power, VRAM; crossed threshold flashes the panel border and names the breach; missing reading never alerts; every device judged |
| FR13 | **Optional session recording** to CSV or JSON Lines via `--log PATH`; un-openable target disables recording with a warning, never stops the monitor |
| FR14 | `--blocks` / `--no-blocks` force the per-block panel on or off, overriding `blocks.enabled`; the two are mutually exclusive (stdlib `flag` has no such group) |

---

## 5. Kernel support

Minimum **5.19** (fdinfo accounting keys). `gpu_metrics` since 5.10; before
it, scattered sysfs attributes (degraded, one at a time). APU notes: VRAM is
a carve-out (lead with GTT), `mem_info_vram_vendor` reads `N/A` (cheap iGPU
hint), v2.x metrics layout is APU-shaped, no fan ever. iGPU detection is a
multi-signal heuristic + config override (DESIGN.md).

---

## 6. `gpu_metrics` binary format

4-byte header, little-endian: `structure_size u16` (total incl. header),
`format_revision u8`, `content_revision u8`. Body at offset 4.
Temperatures in whole °C (`u16`), power in whole W (`u16`), clocks in whole
MHz (`u16`). Sentinels `0xFFFF` / `0xFFFFFFFF` / `0xFFFF_FFFF_FFFF_FFFF` →
`nil`, plus per-family plausibility floors. `structure_size` bounds the body:
larger driver structs truncate safely, smaller ones yield `nil` tails,
unknown `(format, content)` — incl. any `3.x` — falls back to sysfs with
`metrics ABI v{fmt}.{rev} unrecognised` in the footer. Supported: v1.0, v1.1,
v1.2, v1.3, v2.0, v2.1, v2.2 (exact layouts in DESIGN.md §6).

---

## 7. Privilege requirements

All device metrics, hwmon, clocks/DPM/profile/overdrive: **no root** (mode
0444). Per-block: **no root for gputop** (child's privilege is radeontop's).
Own-user processes: no root. Other users' processes: root required — the UI
states the truncation explicitly. gputop never opens the DRM node (0660
root:render); `--blocks` only stats the path for `-p`.

---

## 8. User interface

Panels: header (identity, BDF, driver, index), utilisation (busy % braille),
memory (VRAM/GTT bars; GTT leads on iGPU), clocks (sclk/mclk vs max),
sensors (3 temps, power vs cap, fan, perf level, profile, throttle, PCIe),
process table, footer (interval, sources, hints, warnings). Optional:
power-profile/overdrive (independent of blocks) and blocks (supervised
child). Responsive: stack < 72 cols, table yields < 20 rows, memory graphs
hidden < 28 rows, utilisation graph never dropped. 30 key bindings (see
README §Keys). Overlays: filter prompt, kill confirm (SIGTERM/SIGKILL +
meanings), help, GPU picker. Themes: default/dracula/gruvbox + monochrome
(`--no-color`); 255-stop truecolor ramp, 256-colour xterm-cube snap.

---

## 9. Session recording

`--log PATH`: suffix picks format (`.csv`/`.json`), trailing `.zst`/`.zstd`
picks compression. Append mode, header seeded from existing content, flushed
per record, closed on every exit path. Absent reading = empty cell (never
`0`, never `N/A`). `log.zstd_level` sets the level; `log.interval_s` thins
the stream (`0` = every sample). Un-openable target → warning + recording
disabled, monitor unaffected.

---

## 10. Performance targets

| Target | Requirement |
|--------|-------------|
| Sampler goroutine | never blocks the UI; never sleeps past the interval; interruptible |
| `/proc` scan cost | well under 20 ms for ~800 processes |
| UI redraw | decoupled from sampler; no redraw work on the sampler goroutine |
| Memory | fixed-size rings; no unbounded growth over multi-day runs |
| Startup | first frame < 500 ms |

---

## 11. Acceptance criteria (adapted: no Textual Pilot → golden tests)

1. `go test ./...` green, incl. fixtures for `gpu_metrics` **v1.0–v2.2**, a
   truncated buffer, an unknown `3.x` header, and the **real 120-byte Navi21
   blob**.
2. All sentinels decode to `nil`.
3. The real blob decodes to `edge=51 °C`, `hotspot=58 °C`, `socket=33 W`,
   `uclk=1000 MHz`, `width=16`, `speed=80` → 8.0 GT/s Gen3.
4. Removing any file from the fake sysfs tree, or chmod `0000`, → `N/A`, no panic.
5. Same `drm-client-id` + `drm-pdev` collapse to one row; different ids do not.
6. A static test asserts no sysfs write path exists.
7. `gofmt`/`vet`/`golangci-lint` clean.
8. `--dump` reproduces every value shown in the TUI (no hidden UI-only state).
9. No-radeontop / unprivileged-radeontop / unreadable-card → start fine,
   blocks panel absent with the status line naming the state.
10. A dump line missing `tc`/`smx`/`cr`/`uvd` parses with those blocks missing.
11. Both `pp_od_clk_voltage` eras parse; a 2104 MHz ceiling never shows as voltage.
12. An interrupted `--log` leaves a readable file (CSV one header, JSONL
    complete lines).
13. Crossing then clearing a threshold flashes the border then stops cleanly.
14. Golden UI tests pin all 9 panels at 60/80/120/200 cols and the responsive
    breakpoints; key-model tests cover all 30 bindings.
