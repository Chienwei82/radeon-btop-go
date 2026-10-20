# Development with podman

The host machine deliberately has **no Go toolchain**. Every compile, test,
vet, lint and release runs inside the podman dev image, defined by the `dev`
stage of `Containerfile` and pinned to `golang:1.27.2-bookworm` plus
`golangci-lint v2.14.0`.

That is not a suggestion: `scripts/podev.sh` is the only sanctioned entry point
(see `AGENTS.md` §1). Do not install Go on the host, and do not run
`go build`/`go test` outside the container.

---

## Prerequisites

- `podman` (rootless is fine for all build/test/lint work)
- `bash`, `git`, `tar`, `sha256sum`
- a terminal with at least 60×20 cells if you want to run the TUI

Nothing else. `./scripts/podev.sh image` must be the first command: it pulls
the Go image and installs the pinned linter into it. The other commands reuse
that image and fail if it has never been built.

---

## First run

```bash
git clone <this repo>
cd radeon-btop-go

./scripts/podev.sh image     # build/refresh gputop-dev:1.27.2 (one time)
./scripts/podev.sh build     # compile ./gputop
./gputop                     # run the binary on the host kernel interfaces
```

`build` produces the deliverable binary in the repository root. Running it on
the host (rather than in the container) is intentional for day-to-day TUI work:
the binary is static, and the host sees its own `/proc` without a user
namespace in the way.

---

## The wrapper

```bash
./scripts/podev.sh build        # go build -o gputop ./cmd/...
./scripts/podev.sh test         # go test ./...
./scripts/podev.sh test ./internal/ui/   # one package
./scripts/podev.sh vet          # go vet ./...
./scripts/podev.sh lint         # golangci-lint run ./...
./scripts/podev.sh fmt          # gofmt -l .  (empty output == clean)
./scripts/podev.sh shell        # interactive bash in the dev image
./scripts/podev.sh run [args]   # run the TUI in podman against this host
./scripts/podev.sh screenshots  # regenerate docs/screenshots/*.svg
./scripts/podev.sh image        # rebuild the dev image
```

`test`, `vet` and `lint` append `./...` when no package is named, and forward
flags such as `-count=1` without falling back to the module root. Build and
module caches live in the named volumes `gputop-gocache` and
`gputop-gomodcache`, never in `$HOME` or the repository.

### Running the TUI inside podman

`./scripts/podev.sh run` starts `go run ./cmd/gputop` in the dev image with the
host's kernel interfaces mounted read-only:

```bash
podman run --rm -it \
  --security-opt label=disable \
  --pid=host \
  -v "$PWD:/work:z" -w /work \
  -v /sys:/sys:ro -v /proc:/proc:ro -v /dev/dri:/dev/dri:ro \
  -v gputop-gocache:/gocache -v gputop-gomodcache:/gomodcache \
  gputop-dev:1.27.2 go run ./cmd/gputop
```

Pass any gputop flag through the wrapper:

```bash
./scripts/podev.sh run --check
./scripts/podev.sh run --dump --pretty
./scripts/podev.sh run --no-processes
```

The wrapper adds `-it` only when stdin and stdout are terminals, so piping
`./scripts/podev.sh run --check | less` still works.

**Rootless caveat:** a container in a user namespace cannot read every host
process's `/proc/<pid>/fdinfo`. Device metrics are complete; the process table
may be reported as partial or missing. For full per-process work, build with
podman and run `./gputop` on the host. `--no-processes` is the fastest way to
smoke-test the TUI in a container.

---

## The gates

Before any commit, all of these must be clean **inside the container**:

```bash
./scripts/podev.sh fmt          # prints nothing
./scripts/podev.sh vet
./scripts/podev.sh lint
./scripts/podev.sh test
```

CI runs the same chain from `.github/workflows/ci.yml`, plus
`scripts/check_packaging.sh`, the read-only guard and the reference parity
gate. A stale image is not accepted: CI rebuilds `gputop-dev` on every run.

### Race detector

The dev image sets `CGO_ENABLED=0`, which the race detector needs. Run the race
suite with a direct `podman run`, not through the wrapper:

```bash
podman run --rm -e CGO_ENABLED=1 \
  -v "$PWD:/work:z" -w /work \
  -v gputop-gocache:/gocache -e GOCACHE=/gocache \
  -v gputop-gomodcache:/gomodcache -e GOMODCACHE=/gomodcache \
  gputop-dev:1.27.2 go test -race ./...
```

### UI goldens

`TestScreenGolden` compares the pure screen against
`internal/ui/testdata/screen_100x40.golden`. To regenerate it, run the test
*inside* the container with the environment variable passed to `podman run`; the
wrapper forwards arguments, not the environment, so
`UPDATE_GOLDEN=1 ./scripts/podev.sh test` silently does nothing:

```bash
podman run --rm -v "$PWD:/work:z" -w /work \
  -v gputop-gocache:/gocache -e GOCACHE=/gocache \
  -v gputop-gomodcache:/gomodcache -e GOMODCACHE=/gomodcache \
  -e UPDATE_GOLDEN=1 gputop-dev:1.27.2 \
  go test -run TestScreenGolden ./internal/ui/
```

Read the diff before keeping it. A regenerated golden pins *current* behaviour;
it does not prove parity.

### Parity with the Python reference

`internal/cli/parity_test.go` replays the reference's `--dump` over a synthetic
sysfs/procfs tree under `internal/cli/testdata/parity/` and compares the two
documents field for field, key order included. The tree and golden are
regenerated with:

```bash
/path/to/radeon-btop-py/.venv2/bin/python scripts/parity_fixture.py
```

The reference checkout is read-only source of truth; never modify it.

---

## Screenshots

The README images are generated, not pasted. `tools/screenshots` builds a
deterministic synthetic machine (an RX 6800 and a Cezanne APU, five DRM
clients, a minute of history, blocks and SCPP tables), calls the same
`ui.RenderScreen` the TUI uses, and writes SVG:

```bash
./scripts/podev.sh screenshots
```

Output:

| File | What it shows |
|---|---|
| `docs/screenshots/main.svg` | 120×48 dashboard: graphs, sensors, process table, power profile and blocks |
| `docs/screenshots/process-tree.svg` | the same machine with `t` (tree) on |
| `docs/screenshots/multi-gpu.svg` | `o` overview: dGPU and APU at once |
| `docs/screenshots/narrow.svg` | a 64-column terminal: sensors stack under the graphs |
| `docs/screenshots/help.svg` | the `h` key reference, centred on its own canvas |

The generator is deliberately synthetic: a screenshot taken from whatever the
build machine was running is not reproducible, and a graph full of desktop
noise is not a fair picture of the interface. SVG keeps braille and bar
characters sharp at any zoom and diffs like text. Re-run the command whenever a
panel changes; the images cannot drift from the renderer.

---

## Packaging and release checks

```bash
./scripts/check_packaging.sh   # flags, themes and version agree across man/completions/PKGBUILD
./scripts/release.sh           # static binary + deterministic tarball + sha256 under dist/
```

`check_packaging.sh` parses the flag registrations in `cmd/gputop/main.go`, the
theme names in `internal/model/theme.go` and the version in four places. A
missing option in the man page or a completion fails the gate before a package
is built.

`release.sh` builds the `release` stage of the `Containerfile`, copies `/gputop`
out of the distroless image, verifies `--version` both from the extracted
binary and from the image, and writes
`dist/gputop-<version>-linux-<arch>.tar.gz` plus a deterministic `.sha256`.
Set `GPUTOP_SOURCE_DIR` to build from a `git archive` extraction instead of the
working tree; that is how the clean-checkout criterion is verified.

---

## Where things live

```
cmd/gputop/            flag parsing and dispatch (no domain logic)
internal/model/        pure data; nil means N/A, never zero
internal/readers/      sysfs, procfs, gpu_metrics, radeontop child, merge
internal/sampler/      one tick goroutine, bounded channel, history ring
internal/ui/           bubbletea v2 event loop + pure render functions
internal/config/       YAML load/validate + atomic state: write-back
internal/check/        the --check report
internal/sessionlog/   CSV/JSONL/zstd recording
internal/control/      signal refusal rules and confirmation
tools/screenshots/     README image generator
packaging/             man page, completions, Arch PKGBUILD
```

`SPEC.md`, `DESIGN.md` and `PROGRESS.md` are the living specification and
verification ledger of the port; update them when behaviour changes.

---

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `exec: "go": executable file not found` | You ran Go on the host. Use `./scripts/podev.sh`. |
| `no Go files in /work` | An argument-less flag reached `go test`; the wrapper now appends `./...`, but name a package if you pass unusual flags. |
| `the input device is not a TTY` | `podev.sh run` was piped without a terminal; remove `-it` by not running the TUI, or use `--check`/`--dump`. |
| The TUI shows an empty process table in a container | Rootless user namespace hides other users' `/proc/<pid>/fdinfo`; run `./gputop` on the host, or accept the partial view. |
| `UPDATE_GOLDEN=1 ./scripts/podev.sh test` changes nothing | The wrapper forwards arguments, not the environment; use the direct `podman run` command above. |
| `permission denied` mounting `/sys` | Add `--security-opt label=disable` on SELinux hosts; all mounts are `:ro`. |
| A source change is not compiled | Run `./scripts/podev.sh image` after editing the `Containerfile`; otherwise the cached dev image is reused on purpose. |

---

## Workflow rules

- Work on a branch; `main` is protected. Open a PR and merge it yourself.
  Never force-push `main`.
- Commit messages are English, imperative and scoped by phase, e.g.
  `phase4: add README screenshots and deployment docs`.
- TDD is non-negotiable for production code: a failing test first, then the
  implementation, then refactor.
- `memory-bank/` and `.clinerules/` are local agent memory. They are ignored by
  git and must never be committed or pushed.
