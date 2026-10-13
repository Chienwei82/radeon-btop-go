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
        run_in_dev go test "${@:-./...}"
        ;;
    vet)
        run_in_dev go vet "${@:-./...}"
        ;;
    lint)
        run_in_dev golangci-lint run "${@:-./...}"
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
    help|*)
        sed -n '2,/^#$/p' "${BASH_SOURCE[0]}" | sed 's/^# \?//'
        exit 1
        ;;
esac
