# gputop-go — Design Document

**Companion to:** [SPEC.md](SPEC.md)
**Status:** Living design of the Go port. Kernel-interface claims are inherited
as verified on live hardware (Navi 21 / RX 6800, RDNA2, `amdgpu` 6.19.4,
kernel `7.0.0-34-generic`); claims that could **not** be verified are marked
`UNVERIFIED` and treated as risks. Deltas vs the Python DESIGN.md are called
out explicitly.

---

## 1. Architecture

```
                          ┌──────────────────────────────────────────────┐
                          │              TUI goroutine (bubbletea)       │
                          │                                              │
                          │  header graphs memory sensors table footer  │
                          │       │  tick msg → drain newest           │
                          └───────┼──────────────────────────────────────┘
                                  │ chan Snapshot (cap 1–2, drop stale)
                   immutable      ▼
                   Snapshot  ┌────────────────────────────┐
                             │   Sampler goroutine        │
                             │   (only mutable owner)     │
                             │  prev_engine_ns map        │ ← sampler-private
                             │  prev_fdinfo cache        │   delta state
                             └─────────────┬──────────────┘
                                           │ read-only sysfs/procfs
      ┌────────────────────────────────────┼───────────────────────┐
      ▼                                    ▼                       ▼
 ┌──────────────┐                 ┌─────────────────┐      ┌────────────────┐
 │ gpu_metrics  │                 │ hwmon + device  │      │ /proc/*/fdinfo │
 │ (binary)     │── metricsbin ──▶│ sysfs readers   │      │ scanner        │
 └──────────────┘                 └────────┬────────┘      └───────┬────────┘
                                  ┌────────┴──────────┐            │
                                  │  merge: binary ▸  │            │
                                  │  sysfs ▸ hwmon ▸  │            │
                                  │  dpm              │            │
                                  └───────────────────┘            │
```

### Go package map (mirrors Python modules)

| Python | Go | Notes |
|---|---|---|
| `model/` dataclasses | `internal/model` structs | frozen→by-value; `X\|None`→pointers |
| `readers/fsutil` | `internal/readers/fsutil` | `(value, ok)` returns, never panic |
| `readers/discovery,hwmon,dpm,throttle` | same names under `internal/readers` | identical paths/units/quirks |
| `readers/metrics_bin` | `internal/readers/metricsbin` | manual offsets+padding, `encoding/binary` |
| `readers/procfs` | `internal/readers/procfs` | raw syscalls, string paths in hot loop |
| `readers/pp` | `internal/readers/pp` | two-era parser, unit-carrying entries |
| `readers/radeontop` | `internal/readers/radeontop` | `exec.CommandContext`, both pipes |
| `sampler/engine` | `internal/sampler` | goroutine + chan + `atomic.Pointer` view |
| `ui/app,panels,…` | `internal/ui` | bubbletea Model + pure `string` builders |
| `cli,config,check,control,state,sessionlog` | `internal/cli,config,check,control,state,sessionlog` | `flag` stdlib; hand-rolled `[state]` |

### Go deltas vs Python (deliberate)

- Thread + `SimpleQueue` → goroutine + `chan Snapshot` (cap 1–2, non-blocking
  send drops stale). No GIL split needed; race detector (`-race`) in CI
  replaces the free-threaded qualification job.
- Textual + Rich → bubbletea v2 + lipgloss + bubbles. No `Pilot` equivalent:
  UI tests are golden tests over pure render functions + key-model tests.
- `tomllib` + `tomli_w` → BurntSushi/toml (read) + hand-rolled `[state]`
  section replacement (comment preservation; no Go writer keeps comments).
- `argparse` → stdlib `flag` + manual `--blocks/--no-blocks` exclusion.
- `compression.zstd` → klauspost/compress (pure Go).
- `os.kill`/`pwd` → `syscall.Kill`/`os/user`; CPU meter via
  `CLOCK_PROCESS_CPUTIME_ID`.
- `mypy --strict` + ruff → `go vet` + `golangci-lint` + `gofmt`.

---

## 2. Concurrency design

One sampler goroutine owns: sequence counter, history rings per device,
per-`(bdf, client_id)` engine baselines, warning sets (cleared wholesale at
256 entries), stats. It publishes `Snapshot` values (slices treated as
read-only, maps never shared) on a channel of capacity 1–2: the send is
non-blocking so a slow UI drops stale frames instead of queueing them.

The tick loop runs on monotonic deadlines (`time.Now` monotonic clock):
`skipped = (now - deadline) / period + 1` dropped slots on overrun, phase
preserved, sleep clamped to one period and interruptible via context. The
interval is re-read live so `+`/`-` take effect at once. `sampleOnce` refuses
to run beside the tick goroutine (error, not panic). A failing device is
counted (`read_errors`) and dropped from that snapshot; a failing tick is
counted (`tick_errors`) and the loop continues.

The TUI drains with non-blocking receive on each tick message; render buffers
are rebuilt per frame on the TUI goroutine alone. The one deliberate I/O
outside the sampler is `control.Terminate` behind the `k` key: a single
non-blocking `kill(2)` behind a disabled-by-default flag + confirmation.

## 3. Data shapes (model-first, MDD)

`internal/model` ports every dataclass field-for-field: `AmdgpuDevice`
(index, card, bdf, name, kind igpu/dgpu, vram vendor, driver version,
boot VGA), `AmdgpuMetrics` (busy %, pools, clocks+source, temperatures,
power, fan, perf level, profiles, overdrive, throttle, PCIe, blocks),
`GpuProcess` (pid, name, user, bdf, client_id, engine buckets, vram/gtt,
`engine_percent` = 0.0 baseline-less by design), `GpuSnapshot` (sequence,
monotonic + wall timestamps, devices, processes, visible/total counts,
source report, warnings ≤ 8, stats), `Thresholds`/`AlertState`, `GpuBlocks`
+ `BlocksStatus`, ring buffers. Pointers for every optional scalar.

## 4. Kernel interface notes (inherited, verified)

- `gpu_metrics` header `<HBB`; body at offset 4; v1.0–v2.2 layouts with
explicit offsets; sentinels → `nil`; plausibility bounds per family
(MAX_CLOCK 10000, MAX_TEMP 150, MAX_POWER 10000, voltage 50–2000 mV).
- DPM units written `Mhz` (lowercase h); `current_sclk` absent on recent
drivers → fall back to `*`-marked `pp_dpm_*` row.
- `drm-engine-*` counters carry a trailing ` ns`; memory units are `KiB`
(IEC); `drm-pdev` matches the discovered BDF.`
- hwmon label matching: `edge`/`junction`/`hotspot`/`mem`, VRM rails kept as
hwmon-only sensors; `fan1_max` derives duty; absent fan ≠ stopped.
- SCPP `pp_power_profile_mode`: full table + `*` active; detail rows open
with digit+`(` and are skipped by the same pattern.
- `pp_od_clk_voltage`: no `OD_*` header → single legacy voltage table;
sections `OD_SCLK`/`OD_MCLK`/`OD_RANGE`/`OD_VDDGFX_OFFSET` with per-entry
units (`500Mhz`, `-50mV`); 2104 MHz must never render as voltage.
- radeontop diagnostics split across **both** stdout ("are you root?") and
stderr; classify from the pair; `-p` by DRM node path (bus numbers are
ambiguous); `-i` is whole seconds (`atoi`).

## 5. TUI design (bubbletea)

`Update` handles `tea.KeyPressMsg` (30 bindings), `tea.WindowSizeMsg`
(responsive breakpoints 72/20/28), tick messages (drain snapshot → rebuild
render buffers → record sample), flash-timer messages (alert borders). `View`
frames pure `string` builders with lipgloss (borders, joins, color profiles).
Process table: bubbles table fed by `procview`-equivalent pure logic (sort,
filter, tree, idle-hiding, selection by identity, top-marker `▶`). Braille:
`BRAILLE_BITS` verbatim, gaps preserved, per-dot-column accumulation, golden
tests at explicit sizes. Themes: 3 palettes + monochrome; 255-stop truecolor
ramp (odd count → 50% lands on mid), 25-stop 256-colour cube-snapped ramp.
Overlays (filter/kill/help/picker) as stacked sub-models returning values.

## 6. Open risks (`UNVERIFIED`)

- `format_revision == 3` (RDNA3): no sample, no published struct →
unrecognised, sysfs fallback (SPEC §6). Same stance as the reference.
- bubbles table API surface vs the 30-binding + identity-selection model:
confirm via Context7 at Phase 2 start; fallback is a hand-rolled renderer
(the pure-logic split makes this cheap).
- Terminal capability detection for truecolor vs 256-colour in lipgloss:
confirm at Phase 2; the cube-snap path must keep the ramp monotonic.
- Upstream `gpu_metrics` encoding rewrites in flight: the registry design
(two-byte key + one layout table per version) keeps additions one-entry.
