# Deploying gputop

gputop is a **client-side monitor**, not a service. "Deploy" means putting one
static binary, one package, or one container image on the machine that has the
GPU. There is no daemon to enable, no port to open, no database, and no
system-wide configuration to write.

The application only reads kernel files. It never writes to sysfs/procfs, never
changes clocks or power limits, and needs no setuid helper.

---

## What you need on the target machine

| | |
|---|---|
| Driver | in-tree `amdgpu` (the older `radeon` driver is not supported) |
| Kernel | 5.19+ for the per-process table; 5.10+ for the `gpu_metrics` table |
| Privileges | an ordinary user is enough for device metrics. Other users' processes are only visible with root (or a permissive `procfs`), and gputop says so instead of guessing |
| Optional | `radeontop` on `$PATH` for the per-block panel (`--blocks`) |
| Runtime | none. The binary is static; no Python, Go, ROCm or Mesa |

Run `gputop --check` first on a new machine: it names every missing metric and
the reason (kernel too old, APU without a fan, permission, radeontop absent).

---

## Option 1 — release tarball (recommended)

The release artifact is `gputop-<version>-linux-<arch>.tar.gz` plus a
`.sha256`. It contains the binary, the man page, the three shell completions,
`gputop.example.yaml`, the README, `docs/`, the living docs (`SPEC.md`,
`DESIGN.md`, `PROGRESS.md`), `LICENSE` and `NOTICE`.

### Build the artifact from a tag

The repository has no published remote yet, so the reproducible path is a local
build from a tag. It uses podman only; the host never receives a Go toolchain.

```bash
git archive --format=tar.gz --prefix=gputop-1.0.0/ \
  -o /tmp/gputop-1.0.0-src.tar.gz v1.0.0
mkdir -p /tmp/gputop-build
tar -xzf /tmp/gputop-1.0.0-src.tar.gz -C /tmp/gputop-build

GPUTOP_SOURCE_DIR=/tmp/gputop-build/gputop-1.0.0 \
  ./scripts/release.sh
```

The script builds the `release` stage of the `Containerfile`, extracts
`/gputop` from the distroless image, checks that the extracted binary and the
image both report the version, and writes the tarball plus its checksum under
`/tmp/gputop-build/gputop-1.0.0/dist/`.

### Install it

```bash
cd /tmp
# download gputop-1.0.0-linux-x86_64.tar.gz and its .sha256 into /tmp first
sha256sum -c gputop-1.0.0-linux-x86_64.tar.gz.sha256
tar -xzf gputop-1.0.0-linux-x86_64.tar.gz
cd gputop-1.0.0-linux-x86_64

./gputop --version                 # gputop 1.0.0
./gputop --check
```

A system-wide install:

```bash
sudo install -Dm755 gputop /usr/local/bin/gputop
sudo install -Dm644 packaging/man/gputop.1 /usr/local/share/man/man1/gputop.1
sudo install -Dm644 packaging/completions/gputop.bash \
  /usr/local/share/bash-completion/completions/gputop
sudo install -Dm644 packaging/completions/gputop.zsh \
  /usr/local/share/zsh/site-functions/_gputop
sudo install -Dm644 packaging/completions/gputop.fish \
  /usr/local/share/fish/vendor_completions.d/gputop.fish
```

Or a user install without root:

```bash
install -Dm755 gputop ~/.local/bin/gputop
install -Dm644 packaging/man/gputop.1 ~/.local/share/man/man1/gputop.1
```

Put the example configuration in place only if you want to change a default:

```bash
mkdir -p ~/.config/gputop
cp gputop.example.yaml ~/.config/gputop/gputop.yaml
```

There is no service unit to install: run `gputop` in a terminal. If you want a
desktop launcher, point it at `gputop` and a terminal emulator; gputop needs a
pty with colour and at least 60 columns by 20 rows.

### Verify the deployment

```bash
gputop --devices               # discovered GPUs as JSON
gputop --check                 # why a metric is missing
gputop --dump --pretty | less  # every value the TUI would show
```

Exit status 3 from `--check`/`--devices` means "no AMD GPU found", which is a
machine fact rather than a broken install.

---

## Option 2 — Arch Linux package

The recipe lives in the source tree, at `packaging/arch/PKGBUILD`. Build it
from a checkout:

```bash
cd packaging/arch
makepkg -si
```

`makedepends=('podman')` is intentional: the recipe rebuilds the pinned dev
image and compiles inside it, so `podman` is the build dependency, not a Go
toolchain. The package installs:

- `/usr/bin/gputop`
- `/usr/share/man/man1/gputop.1`
- bash, zsh and fish completions
- `LICENSE` and `NOTICE` under `/usr/share/licenses/gputop/`

The recipe currently has `source=()`: it packages the tree it lives in. Add a
release tarball URL once the project remote is settled.

---

## Option 3 — container image

The `Containerfile` has a `release` stage that produces a distroless image with
`/gputop` as its entrypoint. Build it locally:

```bash
podman build --target release --build-arg VERSION=1.0.0 -t gputop:1.0.0 .

podman run --rm gputop:1.0.0 --version
podman run --rm gputop:1.0.0 --help
```

### Run it against the host's kernel interfaces

The image contains no kernel interfaces of its own. Mount the host's `/sys`,
`/proc` and `/dev/dri` **read-only**; `/dev/dri` is only needed so `--check` can
list the render nodes (gputop does not open the device nodes for metrics).

```bash
podman run --rm -it \
  --security-opt label=disable \
  --pid=host \
  -v /sys:/sys:ro \
  -v /proc:/proc:ro \
  -v /dev/dri:/dev/dri:ro \
  gputop:1.0.0
```

Headless check with the same mounts:

```bash
podman run --rm \
  --security-opt label=disable --pid=host \
  -v /sys:/sys:ro -v /proc:/proc:ro -v /dev/dri:/dev/dri:ro \
  gputop:1.0.0 --check
```

`--pid=host` is what makes `/proc` show the host's process list rather than the
container's. `--security-opt label=disable` keeps SELinux from relabelling the
host's `/sys` and `/proc`; the mounts are read-only regardless.

**Process-table caveat, measured:** a rootless container runs in a user
namespace and cannot inspect the host's other users' `fdinfo` entries. On the
test machine `--check` reported the table as missing (595 of 597 processes
belonged to another user), while device metrics were complete (7/7). Use the
native binary when you need the full per-process table, or run the container
with enough privilege for your policy. gputop reports the partial view rather
than pretending the table is complete.

The image is distroless: no shell, no package manager, no `podman exec`.
`docker run` works the same way if that is what the host uses.

The image is **not published to a registry yet** (there is no project remote).
Build it locally or push it to your own registry with `podman push`; the
`Containerfile` labels already carry the OCI title, version and licence.

---

## Option 4 — from a checkout, with podman only

This is the developer path; see [DEVELOPMENT.md](DEVELOPMENT.md) for the full
workflow.

```bash
git clone <this repo>
cd radeon-btop-go
./scripts/podev.sh image     # build the pinned dev image (first run only)
./scripts/podev.sh build     # produces ./gputop using the container toolchain
./gputop
```

The host still has no Go toolchain. `podev.sh build` writes the binary into the
checkout; nothing is installed outside it.

---

## Uninstall

```bash
# tarball / user install
rm -f ~/.local/bin/gputop ~/.local/share/man/man1/gputop.1

# system-wide tarball install
sudo rm -f /usr/local/bin/gputop /usr/local/share/man/man1/gputop.1
sudo rm -f /usr/local/share/bash-completion/completions/gputop
sudo rm -f /usr/local/share/zsh/site-functions/_gputop
sudo rm -f /usr/local/share/fish/vendor_completions.d/gputop.fish

# Arch
sudo pacman -R gputop

# container
podman rmi gputop:1.0.0
```

gputop writes only its own configuration/state file
(`~/.config/gputop/config.yaml`, or `$GPUTOP_STATE`) and a recording when
`--log` is used. Remove those if you want a clean uninstall.

---

## What "deployed" does not mean here

- No background sampler, no metrics daemon, no exporter, no HTTP endpoint.
- No root requirement for device metrics and no privileged helper.
- No write path to the GPU: power profiles and overdrive tables are displayed,
  never applied. This is a tested property, not a promise (`internal/readers`
  is scanned in CI for filesystem mutators and write-capable opens).
