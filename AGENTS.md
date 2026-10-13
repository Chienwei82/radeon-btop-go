# AGENTS.md — rules for AI agents working in this repository

This file is the contract between the human owner and any coding agent.
Read it before doing anything else in this repo.

## 0. Memory first

- Before starting any task, read ALL files in `memory-bank/`:
  `projectbrief.md`, `productContext.md`, `systemPatterns.md`, `techContext.md`,
  `phases.md`, `activeContext.md`, `progress.md`.
- After finishing a task or phase, update `activeContext.md` and `progress.md`.
  A full documentation review happens on explicit request ("update memory bank").
- `memory-bank/` and `.clinerules/` are local agent memory: never commit them
  (see `.gitignore`), never push them.

## 1. Host stays clean

- The host machine has NO Go toolchain by design. All Go work runs inside
  podman containers via `scripts/podev.sh`:
  - `./scripts/podev.sh build` — compile `cmd/gputop`
  - `./scripts/podev.sh test` — `go test ./...`
  - `./scripts/podev.sh vet` — `go vet ./...`
  - `./scripts/podev.sh lint` — `golangci-lint run ./...`
  - `./scripts/podev.sh shell` — interactive shell in the dev image
- NEVER install Go, linters, or project dependencies on the host.
- NEVER `go run`, `go build`, or `go test` on the host.
- Build/module caches live in the podman named volume `gputop-gocache`,
  never in `$HOME` or the repo.
- The compiled binary is delivered into the repo (`./gputop` via bind mount),
  never into host system paths.

## 2. Methodology gates (non-negotiable)

- **TDD**: red → green → refactor. No production code without a failing test
  first, except for pure scaffolding (Containerfile, CI, packaging).
- **MDD**: `internal/model` is the domain model and leads. `readers`,
  `sampler` and `ui` only consume it. SPEC.md/DESIGN.md/PROGRESS.md are the
  living specification of this port; update them per milestone.
- **Clean code**: small single-responsibility packages; pure functions for
  parsing, formatting and rendering (testable without a terminal); domain
  names (`gpu_metrics`, `fdinfo`, `SCPP`); `nil` means `N/A` — never zero.
- **context7**: consult Context7 docs for every third-party API touched
  (bubbletea, lipgloss, bubbles, toml, zstd, golangci-lint) and record the
  library ID used in `memory-bank/progress.md`.
- Gates before any commit: `gofmt -l .` clean, `vet` clean, `lint` clean,
  `go test ./...` green — all inside the container.

## 3. Domain invariants (inherited from the Python reference)

- **Read-only**: never open a sysfs/procfs path for writing. A CI job greps
  `internal/readers` for write paths (same technique as the original).
- **Honest numbers**: absent reading renders `N/A` / empty cell, never `0`,
  never guessed. Partial `/proc` visibility is reported, not hidden.
- **Unknown `gpu_metrics` revisions** (incl. any `3.x`) fall back to sysfs;
  never guess a layout.
- Merge precedence: `gpu_metrics` ▸ device sysfs ▸ hwmon ▸ DPM tables.
- fdinfo dedup by `(drm-pdev, drm-client-id)` taking per-field maximums,
  never sums. Engine buckets: `gfx`, `compute`, `dma`, `enc`, `dec`, `other`.

## 4. Workflow

- `main` is protected: work on a branch, open a PR, merge it yourself.
  No direct pushes to `main`, no force-push.
- Commit messages in English, imperative mood, scoped by phase
  (e.g. `phase0: add podman dev wrapper`).
- Reference implementation: `/home/david/Codigo/radeon-btop-py/`
  (read-only source of truth for behavior; never modify it).
