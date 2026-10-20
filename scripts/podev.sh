#!/usr/bin/env bash
# podev.sh — the ONLY sanctioned entry point to the Go toolchain.
#
# The host has no Go by design (see AGENTS.md §1). Every command below runs
# inside the podman dev image with the repo bind-mounted at /work.
#
# Build/module caches live in named volumes (gputop-gocache, gputop-gomodcache),
# never in $HOME or the repo, so repeated runs are fast and the host stays clean.
#
# Usage:
#   ./scripts/podev.sh build [packages...]   # go build ./... (binary -> ./gputop)
#   ./scripts/podev.sh test  [packages...]   # go test ./...
#   ./scripts/podev.sh vet   [packages...]   # go vet ./...
#   ./scripts/podev.sh lint  [packages...]   # golangci-lint run ./...
#   ./scripts/podev.sh shell                 # interactive shell in the dev image
#   ./scripts/podev.sh run [args...]         # run the TUI against this host (read-only mounts)
#   ./scripts/podev.sh screenshots           # regenerate docs/screenshots/*.svg
#   ./scripts/podev.sh image                 # (re)build the dev image
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${GPUTOP_DEV_IMAGE:-gputop-dev:1.27.2}"

cmd="${1:-help}"
shift || true

build_image() {
    podman build -f "${REPO_ROOT}/Containerfile" \
        --target dev -t "${IMAGE}" "${REPO_ROOT}"
}

run_in_dev() {
    podman run --rm \
        -v "${REPO_ROOT}:/work:z" -w /work \
        -v gputop-gocache:/gocache -e GOCACHE=/gocache \
        -v gputop-gomodcache:/gomodcache -e GOMODCACHE=/gomodcache \
        "${IMAGE}" "$@"
}

# run_against_host runs a command in the dev image with the host's kernel
# interfaces mounted read-only, so the TUI sees the same GPUs and processes as
# `./gputop` on the host. --pid=host is what makes /proc show the host's
# process list rather than the container's; the mounts are read-only because
# the application has no write path and must never acquire one by accident.
run_against_host() {
    local tty_flags=()
    if [ -t 0 ] && [ -t 1 ]; then
        tty_flags=(-it)
    fi
    podman run --rm "${tty_flags[@]}" \
        --security-opt label=disable \
        --pid=host \
        -v "${REPO_ROOT}:/work:z" -w /work \
        -v /sys:/sys:ro -v /proc:/proc:ro -v /dev/dri:/dev/dri:ro \
        -v gputop-gocache:/gocache -e GOCACHE=/gocache \
        -v gputop-gomodcache:/gomodcache -e GOMODCACHE=/gomodcache \
        "${IMAGE}" "$@"
}

# has_package_arg reports whether any argument names a package to operate on.
# A flag such as -count=1 or -v, or a flag's value, does not: without this,
# `podev.sh test -count=1` forwarded only the flag and `go test` fell back to
# the module root, which holds no Go files ("no Go files in /work").
has_package_arg() {
    local arg
    for arg in "$@"; do
        case "${arg}" in
            .|./*|/*|*...) return 0 ;;
        esac
    done
    return 1
}

case "${cmd}" in
    image)
        build_image
        ;;
    build)
        # No arguments means "produce the deliverable binary", which is a
        # single main package: `-o file ./...` is rejected by the toolchain
        # once there is more than one package to build.
        if [ "$#" -eq 0 ]; then
            run_in_dev go build -o gputop ./cmd/...
        else
            run_in_dev go build "$@"
        fi
        ;;
    test)
        if has_package_arg "$@"; then
            run_in_dev go test "$@"
        else
            run_in_dev go test "$@" ./...
        fi
        ;;
    vet)
        if has_package_arg "$@"; then
            run_in_dev go vet "$@"
        else
            run_in_dev go vet "$@" ./...
        fi
        ;;
    lint)
        if has_package_arg "$@"; then
            run_in_dev golangci-lint run "$@"
        else
            run_in_dev golangci-lint run "$@" ./...
        fi
        ;;
    fmt)
        # gofmt gate (AGENTS.md §2). With no argument it lists files that are
        # not gofmt-clean (empty output == clean), mirroring `gofmt -l .`.
        if [ "$#" -eq 0 ]; then
            run_in_dev gofmt -l .
        else
            run_in_dev gofmt "$@"
        fi
        ;;
    shell)
        podman run --rm -it \
            -v "${REPO_ROOT}:/work:z" -w /work \
            -v gputop-gocache:/gocache -e GOCACHE=/gocache \
            -v gputop-gomodcache:/gomodcache -e GOMODCACHE=/gomodcache \
            "${IMAGE}" bash
        ;;
    run)
        run_against_host go run ./cmd/gputop "$@"
        ;;
    screenshots)
        run_in_dev go run ./tools/screenshots "$@"
        ;;
    help|*)
        sed -n '2,/^#$/p' "${BASH_SOURCE[0]}" | sed 's/^# \?//'
        exit 1
        ;;
esac
