# gputop-go — progress log

**Convention:** updated at the end of every milestone. Each entry records what
was built, what was *verified* (and on what), what was assumed, and what was
left undone. The verification ledger below separates measured properties from
designed-for ones. Phase numbers refer to `memory-bank/phases.md`.

---

## Milestone 0 — toolchain and foundations

**Scope:** Phase 0. Podman toolchain, agent contracts, memory bank, living
spec docs, packaging skeleton.

**Delivered** (this milestone, pending exit verification):

- `Containerfile` (dev + release stages; Go 1.27.2; golangci-lint v2.14.0)
- `scripts/podev.sh` (build/test/vet/lint/shell/image, named-volume caches)
- `go.mod` + `cmd/gputop` skeleton (`--version`)
- `AGENTS.md`, `.clinerules/memory-bank.md`, `memory-bank/` (7 files)
- `README.md`, `SPEC.md`, `DESIGN.md`, `PROGRESS.md` (this file),
  `gputop.example.yaml` (key set shared with the Python build)
- `packaging/` skeleton (next: man, completions, PKGBUILD)

**Verified**

- `go.dev/VERSION` = go1.27.2; Docker Hub pull works; `go version go1.27.2
  linux/amd64` inside podman (2026-10-13).
- Context7 IDs resolved: `/charmbracelet/bubbletea`, `/charmbracelet/lipgloss`,
  `/charmbracelet/bubbles` (v2.0.0), `/goccy/go-yaml`, `/klauspost/compress`.
  Docs queried: bubbletea v2 messages/views, lipgloss tables/joins/profiles,
  YAML decode/comment round-trip.

**Assumed**

- golangci-lint v2.14.0 understands Go 1.27 export data (v2.5.0 did not;
  rebuild pending verification — see exit check below).
- bubbles table API covers identity-selection + 30 bindings (confirm Phase 2).
- klauspost zstd streaming API (confirm Phase 3).

**Exit check:** `podev.sh build/test/vet/lint` green + initial commit.

---

## Verification ledger

| Claim | Status | Evidence |
|---|---|---|
| Go 1.27.2 in podman | measured | `go version` in container, 2026-10-13 |
| Image pull works | measured | `podman pull golang:1.27.2-bookworm` ok |
| Context7 library IDs | measured | resolve calls 2026-10-13 (see above) |
| bubbletea v2 API shape | measured | context7 query (KeyPressMsg/tea.View/WindowSizeMsg) |
| lipgloss tables/profiles | measured | context7 query |
| YAML comment round-trip (AST) | measured | context7 query, 2026-10-13 (`/goccy/go-yaml`) |
| golangci-lint ↔ Go 1.27 | assumed | v2.14.0 rebuild pending |
| gpu_metrics layouts | designed-for | inherited verified claims from Python SPEC §6 |
| fdinfo accounting | designed-for | inherited verified claims from Python DESIGN §7 |
| TUIgolden-test strategy | designed-for | no code yet; Phase 2 |

---

## Notes for future milestones

- Keep editor payloads < 6000 chars per call; use heredocs/`cat >>` for docs.
- Long podman builds: `nohup ... &` + poll the log file; never block 30 s+.
- `BRAILLE_BITS` is the single source of truth for the dot layout (dots 1–3
  and 7 form the LEFT column).
- radeontop splits diagnostics across stdout AND stderr; drain both.
- A fixture whose `drm-engine-*` counters never advance has NO utilisation
  (cumulative counters; deltas need a baseline).
- `nil` = absent everywhere; tests must pin `nil`-vs-zero distinctly.
