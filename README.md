# gputop (Go)

A btop-style terminal monitor for AMD Radeon GPUs driven by the `amdgpu`
kernel driver. It reads what the kernel already publishes — utilisation,
clocks, temperatures, power against the enforced cap, throttle reasons, and
every DRM client with its per-engine time and resident memory — and it never
writes to the GPU.

This is a Go reimplementation of the Python
[gputop](https://github.com/gputop/gputop) with full v1 parity: one static
binary, no interpreter, no runtime dependencies for the user.

Everything on screen comes from files under `/sys/class/drm/card*/device/`
and `/proc/<pid>/fdinfo/`, all of them readable by an ordinary user. There is
no daemon, no setuid helper, and no write path at all: the power-profile and
overdrive tables are displayed, never set.

---

## Contents

- [Install](#install)
- [Supported hardware](#supported-hardware)
- [What each panel shows](#what-each-panel-shows)
- [`gputop --check`](#gputop---check)
- [Options](#options)
- [Keys](#keys)
- [Configuration](#configuration)
- [What it reads](#what-it-reads)
- [Cost: CPU and memory](#cost-cpu-and-memory)
- [Development](#development)

---

## Install

gputop-go ships as a **single static binary**. No interpreter, no runtime.

### From a release

Download `gputop` for your architecture, make it executable, put it on
`$PATH`. (Release process: Phase 4.)

### Arch Linux

A `PKGBUILD` ships with the source, in
[packaging/arch/PKGBUILD](packaging/arch/PKGBUILD):

```bash
cd packaging/arch
makepkg -si          # -s installs build dependencies, -i installs the result
```

It installs the `gputop(1)` man page and the bash, zsh and fish completions,

---

## Supported hardware

gputop talks to the kernel, not to a vendor library, so "supported" means
"the driver publishes the files the panel needs" rather than a list of
marketing names.

| | |
|---|---|
| **Driver** | `amdgpu` (in-tree). A card bound to the older `radeon` driver is not visible to gputop; `gputop --check` says so. |
| **Kernel** | 5.19 or newer — the release that gave `/proc/<pid>/fdinfo` its DRM accounting keys (`drm-client-id`, `drm-engine-*`, `drm-memory-*`), the basis of the process table. |
| **`gpu_metrics`** | 5.10 or newer. Before it, the interface works from scattered sysfs attributes, one number at a time. |
| **Generations** | GCN 1–4, RDNA 1–4, and the APUs of each era. Generation-neutral paths; what appears depends on what the part publishes. |
| **Userspace** | None. gputop never opens a libdrm, Vulkan, ROCm or Mesa handle. |

What differs by generation, and what `--check` tells you on your machine:

- **Fan and extra temperatures.** Discrete cards expose `fan1_input`,
  `fan1_max` and `pwm1`; `temp2_*`/`temp3_*` are a SOC15 dGPU feature. APUs
  are passively cooled; gputop says "not on this GPU generation" rather than
  `N/A`.
- **`gpu_metrics` revision.** The file carries no version string: its
  identity is the four-byte header (`structure_size`, `format_revision`,
  `content_revision`). An unrecognised revision is never guessed at: the
  interface falls back to sysfs and names the revision in use.
- **Overdrive.** `pp_od_clk_voltage` means different things by generation.
  The panel labels what it found rather than assuming one of the three eras.
- **Power profiles.** `pp_power_profile_mode` (SCPP) exists only where the
  driver publishes it. Read-only either way.
- **HBM.** Instinct-class parts report a memory temperature a GDDR6 card does
  not have. gputop shows whichever sensors exist and invents none.

---

## What each panel shows

Nine panels, laid out to survive a narrow or short terminal (stack below 72
columns; process table yields below 20 rows; memory graphs hidden below 28;
the utilisation graph is never dropped).

- **Devices** — every `amdgpu` card, discrete first. `tab` / `shift+tab`
  switch, `1`–`9` jump, `o` shows every GPU at once.
- **Utilisation** — busy %, with the memory panel leading on GTT for an
  integrated GPU.
- **Sensors** — clocks, temperatures, power against the enforced cap, fan,
  throttle reasons, PCIe link width and speed.
- **Processes** — a sortable, filterable table of DRM clients (PID, USER,
  COMMAND, GFX/COMPUTE/DMA/ENC-DEC %, VRAM/GTT). Idle rows hidden unless `a`;
  sort `s`/`S`/`r`, filter `/`, tree `t`, full cmdline `c`.
- **Power profile** *(optional)* — active SCPP profile, the table it was
  chosen from, overdrive ceilings per domain. Read-only, labelled as such.
- **Blocks** *(optional, off by default)* — per-block utilisation from a
  supervised `radeontop` child. Hidden with a reason when unavailable.

---

## `gputop --check`

Reports which metrics this machine can provide and why any are missing:
kernel release vs the 5.19 floor, `gpu_metrics` ABI recognition, per-attribute
presence with the kernel release that introduced it, process-ownership
visibility (own UID vs root), and the radeontop blocks-panel state. `--json`
emits the same report as JSON. Exit status 3 when no AMD GPU is present, so
scripts can distinguish "no hardware" from "gputop is broken".

---

## Options

`gputop [-c PATH] [-i SECONDS] [--drm-root PATH] [--proc-root PATH]`
`[--dump [--pretty]] [--devices] [--kind auto|igpu|dgpu]`
`[--theme NAME] [--no-color] [--no-processes]`
`[--blocks | --no-blocks] [--log PATH] [--check [--json]] [--version]`

- `-c/--config PATH` — config file instead of the default search locations.
- `-i/--interval SECONDS` — sampling interval, overriding
  `general.interval_ms`.
- `--drm-root/--proc-root` — override sysfs DRM dir / procfs mount (testing).
- `--dump` — one JSON snapshot to stdout and exit. `--pretty` indents it.
- `--devices` — list discovered GPUs as JSON and exit.
- `--kind` — force every device to integrated/discrete.
- `--theme/--no-color` — palette control (also in config, also `m`).
- `--no-processes` — skip the `/proc` scan (the expensive part of a sample).
- `--blocks/--no-blocks` — force the per-block panel on/off for one run.
- `--log PATH` — record the session (`.csv`/`.json` by suffix, `+ .zst`
  for zstd). Appended, flushed per record.
- `--check [--json]` — the diagnostic report above.

and offers `radeontop` as an optional dependency for the per-block panel.

### From a checkout (podman only — the host needs no Go toolchain)

```bash
git clone <this repo>
cd radeon-btop-go
./scripts/podev.sh build   # produces ./gputop via the container toolchain
./gputop
```

A man page is in [packaging/man/gputop.1](packaging/man/gputop.1) and
completions in [packaging/completions/](packaging/completions/).

---

## Keys

| Key | Action |
|---|---|
| `q` | quit |
| `tab` / `shift+tab` | next / previous GPU |
| `1`–`9` | jump to GPU n |
| `o` | toggle all-GPUs overview |
| `/` then text | filter processes |
| `esc` | clear filter |
| `h` / `?` | help overlay |
| `R` | resample now |
| `+` / `-` | faster / slower sampling |
| `m` | cycle theme |
| `p` | toggle process table |
| `s` / `S` / `r` | next / previous sort column, reverse direction |
| `a` | toggle idle clients |
| `t` | toggle process tree |
| `c` | toggle full command line |
| `up` / `down` | select previous / next process |
| `k` | signal selected process (needs `process.allow_kill` + confirmation) |

---

## Configuration

Precedence, lowest to highest: built-in defaults → `$GPUTOP_CONFIG` →
`~/.config/gputop/gputop.yaml` → the `state:` block gputop saved for itself →
CLI flags.

Every key is optional; unknown keys are reported as warnings and ignored, so
a file written for a newer release still works today. See
[gputop.example.yaml](gputop.example.yaml) — the key set is the same as the
Python build's, but the format is YAML, so a Python-era `.toml` file is not
read.

`state:` is written by gputop itself (theme, interval, sort, filter, view
choices) into `~/.config/gputop/config.yaml` (`$GPUTOP_STATE` overrides the
path). The write is atomic and edits the parsed document in place, so the rest
of the file — comments and key order included — is preserved.

---

## What it reads

All of it unprivileged, all of it read-only:

- `/sys/class/drm/card*/device/` — VRAM/GTT, busy %, DPM clocks, PCIe link
- `/sys/class/drm/card*/device/gpu_metrics` — versioned binary table (v1.x/v2.x)
- `/sys/class/drm/card*/device/pp_power_profile_mode` — SCPP profile table
- `/sys/class/drm/card*/device/pp_od_clk_voltage` — overdrive ceilings
- `/sys/class/hwmon/hwmon*/` — temperatures, power, fan
- `/proc/<pid>/fdinfo/` — per-process VRAM/GTT and per-engine utilisation

The one thing not read from a file is the optional blocks panel: with
`blocks.enabled` it starts `radeontop` as a child and parses its dump output.
That is the only part that can need privileges. The application never writes
to sysfs — enforced by a CI test that greps `internal/readers` for write
paths.

---

## Cost: CPU and memory

Targets (to be measured for this build in Phase 4 and recorded here):

| | |
|---|---|
| Sampler tick, one device | small sysfs reads only; no TUI framework in the path |
| `/proc` scan | raw syscalls, no per-fd heap churn; well under 20 ms for ~800 processes |
| Running interface | compiled renderer; bounded history; flat memory over multi-day runs |
| Peak RSS | target: well below the Python build's ~78 MiB |

---

## Development

The host needs nothing but podman. All Go work runs in the container:

```bash
./scripts/podev.sh build   # compile ./gputop
./scripts/podev.sh test    # go test ./...
./scripts/podev.sh vet     # go vet ./...
./scripts/podev.sh lint    # golangci-lint run ./...
./scripts/podev.sh shell   # interactive shell in the dev image
```

Methodology gates (see AGENTS.md): TDD (red→green→refactor), MDD
(`internal/model` leads; SPEC/DESIGN/PROGRESS are the living spec), clean
code (`gofmt`+`vet`+`golangci-lint` green before every commit), Context7 for
every third-party API.

See [PROGRESS.md](PROGRESS.md) for the verification ledger: what was measured
on real hardware, what was verified against fixtures, and what is still
assumed.

---

## Licence

GPL-3.0-or-later.
