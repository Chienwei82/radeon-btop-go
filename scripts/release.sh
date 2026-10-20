#!/usr/bin/env bash
# release.sh — build the shippable artifact with podman only.
#
# The host has no Go toolchain (AGENTS.md §1), so the release binary is
# compiled by the `release` stage of the Containerfile, copied out of the
# image, and packaged with the man page, completions, licence and examples.
# The result is a static binary plus a deterministic tarball and checksum
# under dist/ — nothing is installed on the host.
#
# Usage:
#   ./scripts/release.sh [VERSION]
#
# VERSION defaults to the value in cmd/gputop/main.go. GPUTOP_SOURCE_DIR can
# point at a `git archive` extraction, which is how the "clean checkout" exit
# criterion is verified without touching the working tree.
set -euo pipefail

REPO_ROOT="${GPUTOP_SOURCE_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
MAIN="${REPO_ROOT}/cmd/gputop/main.go"
SOURCE_VERSION="$(sed -nE 's/^var version = "([^"]+)"$/\1/p' "${MAIN}")"
VERSION="${1:-${SOURCE_VERSION}}"

if [ -z "${VERSION}" ]; then
    printf 'release: cannot read the version from %s\n' "${MAIN}" >&2
    exit 1
fi
if [ "${VERSION}" != "${SOURCE_VERSION}" ]; then
    printf 'release: warning: building %s while the tree documents %s\n' \
        "${VERSION}" "${SOURCE_VERSION}" >&2
fi

OUT="${REPO_ROOT}/dist"
IMAGE="${GPUTOP_RELEASE_IMAGE:-gputop-release:${VERSION}}"
PODMAN_ARCH="$(podman info --format '{{.Host.Arch}}')"
case "${PODMAN_ARCH}" in
    amd64) ARCH=x86_64 ;;
    arm64) ARCH=aarch64 ;;
    arm) ARCH=armv7h ;;
    *) ARCH="${PODMAN_ARCH}" ;;
esac
NAME="gputop-${VERSION}-linux-${ARCH}"
STAGE="${OUT}/${NAME}"

mkdir -p "${OUT}"
rm -rf "${STAGE}"
mkdir -p "${STAGE}/packaging/man" "${STAGE}/packaging/completions"

printf 'release: building %s for linux/%s\n' "${VERSION}" "${ARCH}"
podman build \
    -f "${REPO_ROOT}/Containerfile" \
    --target release \
    --build-arg VERSION="${VERSION}" \
    -t "${IMAGE}" \
    "${REPO_ROOT}"

cid="$(podman create "${IMAGE}")"
cleanup() { podman rm -f "${cid}" >/dev/null 2>&1 || true; }
trap cleanup EXIT
podman cp "${cid}:/gputop" "${STAGE}/gputop"
chmod 0755 "${STAGE}/gputop"

cp "${REPO_ROOT}/README.md" "${REPO_ROOT}/LICENSE" "${REPO_ROOT}/NOTICE" \
    "${REPO_ROOT}/gputop.example.yaml" "${STAGE}/"
# The living docs the README links to. They are part of the artifact so a
# reader of the tarball can verify the claims without cloning the source.
cp "${REPO_ROOT}/SPEC.md" "${REPO_ROOT}/DESIGN.md" "${REPO_ROOT}/PROGRESS.md" \
    "${STAGE}/"
# A git archive of a pre-docs tag has no docs/; include it when present so the
# README's screenshots and links resolve in the artifact.
if [ -d "${REPO_ROOT}/docs" ]; then
    mkdir -p "${STAGE}/docs"
    cp -R "${REPO_ROOT}/docs/." "${STAGE}/docs/"
fi
cp "${REPO_ROOT}/packaging/man/gputop.1" "${STAGE}/packaging/man/"
cp "${REPO_ROOT}/packaging/completions/"* "${STAGE}/packaging/completions/"

# The staging tree is built with mkdir and cp, and both of those carry the
# builder's umask: with umask 0002 the directories land at 0775 and the copied
# files at 0664, with umask 0022 they land at 0755 and 0644. tar records those
# modes, so without this the checksum of the tarball would depend on who ran
# the build. Every mode is pinned instead, which makes the archive a function
# of the source.
find "${STAGE}" -type d -exec chmod 0755 {} +
find "${STAGE}" -type f -exec chmod 0644 {} +
chmod 0755 "${STAGE}/gputop"

printf 'release: verifying the binary\n'
if command -v file >/dev/null 2>&1; then
    file "${STAGE}/gputop"
fi
if [ "$(uname -m)" = "${ARCH}" ]; then
    if ! "${STAGE}/gputop" --version | grep -Fq "gputop ${VERSION}"; then
        printf 'release: extracted binary does not report version %s\n' "${VERSION}" >&2
        exit 1
    fi
fi
if ! podman run --rm "${IMAGE}" --version | grep -Fq "gputop ${VERSION}"; then
    printf 'release: the release image does not report version %s\n' "${VERSION}" >&2
    exit 1
fi

# Deterministic archive metadata: a rebuild from the same commit produces the
# same tar layout, and the checksum names exactly what was built.
epoch="${SOURCE_DATE_EPOCH:-$(git -C "${REPO_ROOT}" log -1 --format=%ct 2>/dev/null || date +%s)}"
tar --sort=name --mtime="@${epoch}" --owner=0 --group=0 --numeric-owner \
    -C "${OUT}" -czf "${OUT}/${NAME}.tar.gz" "${NAME}"
(cd "${OUT}" && sha256sum "${NAME}.tar.gz" > "${NAME}.tar.gz.sha256")

printf 'release: %s\n' "${OUT}/${NAME}.tar.gz"
printf 'release: %s\n' "${OUT}/${NAME}.tar.gz.sha256"
wc -c "${STAGE}/gputop" "${OUT}/${NAME}.tar.gz"
