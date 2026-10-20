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
| `readers/radeontop` | `internal/readers/radeontop` | `exec.Command` + two drain goroutines (no `CommandContext`: teardown is a polite SIGTERM, then a bounded SIGKILL) |
| `sampler/engine` | `internal/sampler` | goroutine + chan + `atomic.Pointer` view |
| `ui/app,panels,…` | `internal/ui` | bubbletea Model + pure `string` builders |
| `cli,config,check,control,state,sessionlog` | `internal/cli,config,check,control,state,sessionlog` | `flag` stdlib; goccy/go-yaml + AST `state:` write-back |

### Go deltas vs Python (deliberate)

- Thread + `SimpleQueue` → goroutine + `chan Snapshot` (cap 1–2, non-blocking
  send drops stale). No GIL split needed; race detector (`-race`) in CI
  replaces the free-threaded qualification job.
- Textual + Rich → bubbletea v2 + lipgloss + bubbles. No `Pilot` equivalent:
  UI tests are golden tests over pure render functions + key-model tests.
- `tomllib` + `tomli_w` → goccy/go-yaml. YAML rather than TOML because this
  port owes functional parity, not file compatibility, and goccy round-trips
  comments through the AST (`parser.ParseComments` + `Path.ReplaceWithNode`):
  the `state:` block is updated in place instead of being re-emitted, so key
  order and every comment in the user's file survive.
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
units (`500Mhz`, `-50mV`); 2104 MHz must never render as voltage. Domains are
emitted in **file order** (`OD_SCLK` before `OD_MCLK`), never sorted: the
reference's dict preserves first-seen order and the `--dump` parity gate compares
the sequence, not just the set. A section this parser does not model
(`OD_FAN_CURVE`, `OD_VDDC_CURVE`, …) is skipped rather than read as a clock.
- radeontop diagnostics split across **both** stdout ("are you root?") and
stderr; classify from the pair; `-p` by DRM node path (bus numbers are
ambiguous); `-i` is whole seconds (`atoi`).
- Supervised child (Go deltas, all deliberate):
  - `exec.Command` + one drain goroutine per pipe instead of `CommandContext`.
    Teardown is the reference's own shape -- SIGTERM, a bounded wait, then
    SIGKILL -- and `CommandContext` would SIGKILL on cancel, defeating the
    polite path. `Setsid` reproduces `start_new_session=True`.
  - The reaper (`cmd.Wait`) runs only after both drains finish, because Go
    closes a `StdoutPipe` inside `Wait` and reading afterwards is undefined.
    The parent's pipe ends are closed explicitly on replacement and on stop,
    which is what keeps a restarting session descriptor-clean.
  - Failure classification awaits the stderr drain (bounded by
    `StderrDrainTimeoutS`) before reading the captured lines. stdout closes as
    soon as the child dies while stderr is still arriving, so classifying on
    stdout EOF alone reads a partial buffer and reports the *least* actionable
    of radeontop's complaints.
  - `status`/`blocks` are published with `atomic.Pointer`; the restart budget
    lives behind a mutex, because the sampler goroutine starts and polls the
    child while teardown may run elsewhere and that pair genuinely races. The
    sample read path stays lock-free.
  - `argv[0]` is the path `ResolveBinary` accepted, and `BuildArgv` validates
    its options, so a zero value yields radeontop's own defaults rather than
    `-i 0 -t 0`.
  - `WaitForFirstSample` waits out the full `StartupGraceS` even when the
    status is already terminal (no radeontop installed), matching the
    reference; only `--blocks`/`blocks.enabled` pays that cost.



## 5. TUI design (bubbletea)

`Update` handles `tea.KeyPressMsg` (30 bindings), `tea.WindowSizeMsg`
(responsive breakpoints 72/20/28), snapshot messages, and flash-timer
messages. `View` calls the pure `RenderScreen` and then resolves each cell's
theme key to lipgloss once per frame; with `--no-color` it emits the plain
rows, which are also what the goldens read. The cell model keeps the screen a
pure row composition rather than a layout engine: panel frames are drawn
inside that model by `PanelBox`, so the divisions are golden-testable and
every row is still composed at the exact terminal width. A frame is exactly
the width it was given, and bodies on a shared row are padded to one baseline
(`padBody`), so two panels close their bottom borders on the same line.

Cell colour keys are a hex value plus optional `|bold` / `|reverse`
attributes. Both are real style modifiers (`parseCellStyle`), which matters
for the active GPU badge: before the parser existed, `#e6e6e6|reverse` was
handed to lipgloss as a colour and the badge silently lost its highlight.

Key hints use the same split at the row level: `footerHints` paints the key
with the accent and bold, the label stays muted, and the groups are separated
by `·`. The key reference uses the same treatment in a fixed 16-cell column,
so a key never reads as part of its description. `--no-color` keeps the text
separation ("q quit"), which is why the hint line is not a single styled run.

Process table: the pure `procview` logic (sort, filter, tree, idle-hiding,
selection by identity, top-marker `▶`) feeds `RenderTable`; bubbles' table
widget was not adopted because rebuilding rows from a snapshot is cheaper and
the identity-selection rules live in the domain, not the widget. Braille:
`BRAILLE_BITS` verbatim, gaps preserved, per-dot-column accumulation, golden
tests at explicit sizes. Themes: 5 palettes; 255-stop truecolor ramp (odd
count → 50% lands on mid), 25-stop 256-colour cube-snapped ramp.

### The event loop (`internal/ui/app.go`)

One model owns the view state and the snapshot on screen, and nothing else.
The two clocks are bubbletea commands: `waitCmd` blocks (off the render
path) on the sampler's bounded channel, and `flashCmd` is a `tea.Tick` whose
message flips one boolean — the only stateful part of an alert.

- **Selection is by identity** `(drm-pdev, drm-client-id)`. A sample that no
  longer contains the selected client clears the selection; the highlight
  never jumps to whatever row took its place.
- **Overlays own the keyboard** while open (`help`, `filter`, `kill`). The
  kill dialog captures the target when it opens, so a table that reorders
  under the modal cannot signal a different process.
- **One exit path.** `Shutdown` is idempotent: it saves the state, closes the
  recording, and stops the sampler. It runs from `q`, and again after
  `Program.Run` returns, which is what covers a SIGTERM (the signal handler
  asks the program to quit rather than letting the process die).
- **Recording is thinned in the loop**, not in the writer: `log.interval_s`
  is a clock question, and the writer stays a pure sink.

### Recording (`internal/sessionlog`)

CSV and JSON Lines, with zstd selected by the filename suffix (`.zst` or
`.zstd`). The base name decides the format and the suffix decides the
transport, so neither is declared twice. The file is opened in append mode and
the header is seeded from the file's existing size: recording twice to one
path extends it instead of writing a second header into the middle of the
data. A snapshot with several GPUs becomes one row per GPU. Absence is an
empty CSV cell and `null` in JSON, never `0` and never `N/A`. Every failure
is a value (`Problem`), because a recording that takes the monitor down with
it has made things worse.

### Signalling (`internal/control`)

The only code that can end something other than gputop. `Refusal` is pure and
exhaustive (pid <= 0, pid 1, kernel thread, gputop itself), the unknown-signal
fallback is the reversible one, and delivery is a package variable so the
suite can assert what would have been sent without sending it.

### Session state (`internal/config/state.go`, `state_save.go`)

Reading merges the `state:` block under the config (defaults → file → state →
flags). Writing edits the parsed YAML AST in place — comments and key order
survive — then writes a temporary file in the same directory, fsyncs it, keeps
the target's permissions (0600 for a new file), renames it, and fsyncs the
directory. A document that does not parse is **not** written: merging into an
empty document would replace the user's configuration with a bare `state:`.

### Rounding (`internal/model/round.go`)

Python's `round()` is half-to-even and Go's `int(x+0.5)` is half-up, so a bar
fill differed by one cell at exact `.5` boundaries (5% of 10 cells: reference
0, half-up 1). `RoundHalfEven` is the single helper every bar, ramp index,
RGB lerp and PCIe link-speed parse uses; its boundary table is the test.

### Responsive rules (pure, in `internal/ui/layout.go`)

The render layer is a pure function of one `LayoutInput`, so every rule below
is pinned by a test rather than discovered on a terminal:

- The graphs are never what gets dropped. On a short terminal the process
  table yields first (`ShortHeight = 20`); on a narrow one (`NarrowWidth =
  72`) the sensors move *under* the graphs instead of squeezing them away.
  The stacked graph takes two thirds of the height and the sensor frame gets
  the rest, rather than the old title-only strip.
- The wide detail row is the reference's 2:1 split with the sensor column
  held at `max(40, width/3)`: a bar row needs forty columns before its bar is
  too short to read, and the graph spends what is left.
- Memory graphs appear from `MinHeightForMemoryGraphs = 28`, and only when
  the graph column is at least 40 cells wide. VRAM and GTT are two framed
  panels on one row, matching the reference; stacking them spent a title row
  per graph and left half the screen empty. Each honours the same 28-row
  floor, so a short terminal drops the whole pair rather than half of it.
- The optional row (per-block utilisation beside the power profile) has one
  threshold for both panels: `MinHeightForOptionalRow = 38 + 8 = 46`. The
  cost is not the row's height but what it takes from the sensor panel beside
  it, which scrolls: an optional panel must never be the reason a required
  one needs scrolling. That invariant is a test
  (`TestOptionalRowNeverCostsAPrimarySensorReading`).
- A panel with nothing to say is not drawn at all: `optionalRow` returns
  `nil` when both the block sample and the SCPP tables are absent, and `nil`
  is what leaves the main row every line it had. The row is cropped to the
  eight lines the reference measured it at rather than allowed to grow, and
  both framed bodies are padded to the same six-line baseline.
- Below `2 * (MinOptionalPanelWidth + boxOverheadX) + 1` columns the two
  optional panels no longer stack (stacking made the profile invisible once
  the eight-line crop applied). The blocks panel is shown alone: the active
  profile is already named on the sensors panel, while per-block utilisation
  appears nowhere else.
- The alert notice goes last on the status line, so a narrow terminal takes
  the width from the notice rather than from the facts.

## 6. Open risks (`UNVERIFIED`)

- `format_revision == 3` (RDNA3): no sample, no published struct →
unrecognised, sysfs fallback (SPEC §6). Same stance as the reference.
- Upstream `gpu_metrics` encoding rewrites in flight: the registry design
(two-byte key + one layout table per version) keeps additions one-entry.
- `/proc` scan cost: measured ≈ 22 ms at ~800 processes (2026-10-19), ≈ 2 ms
over the SPEC §10 target. The scan is syscall-bound; a raw `getdents` /
`readlinkat` path is the next candidate, not a release blocker.
- ~~bubbles table API vs the 30-binding + identity-selection model~~ Resolved:
  bubbles is not a dependency; `procview` + `RenderTable` are pure functions.
- ~~Terminal capability detection for truecolor vs 256-colour~~ Resolved in
  Phase 2: the gradient snaps to the xterm cube when truecolor is absent.

## 7. Packaging and release (Phase 4)

One static binary, no runtime. The `release` stage of the `Containerfile`
compiles with `CGO_ENABLED=0`, `-trimpath` and `-buildvcs=false`, strips the
binary, and stamps the version with `-X main.version=${VERSION}`. The final
image is `distroless/static-debian12:nonroot`; `.dockerignore` keeps VCS
metadata, agent memory and build output out of the context, so a `git archive`
of a tag is a complete build input.

`scripts/release.sh` is the only release entry point: it builds the image,
copies the binary out of it (the host never gets a Go toolchain), verifies the
version, packages the binary, man page, completions, example config and
licence files into `dist/gputop-<version>-linux-<arch>.tar.gz`, and writes a
deterministic `.sha256`. `scripts/check_packaging.sh` reads the flag names and
themes from the Go sources and fails when the man page or a completion drifts,
or when the version disagrees between the binary, man page, `PKGBUILD` and
`Containerfile`. The Arch recipe builds through podman (`makedepends=('podman')`)
and installs exactly the files the repository carries.

Licence: GPL-3.0-or-later for this port, with the upstream MIT copyright and
licence reproduced in `NOTICE`; both are installed under
`/usr/share/licenses/gputop/`.
