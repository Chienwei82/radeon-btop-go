#!/usr/bin/env bash
# build_appimage.sh — build the gputop AppImage from an already built binary.
#
# The binary is the one scripts/release.sh produced (a static, stripped ELF);
# this script only assembles the AppDir and runs appimagetool over it. The
# tool is downloaded from a pinned release tag and its SHA-256 is verified, so
# the build does not depend on whatever "continuous" happens to be today.
#
# appimagetool needs FUSE to mount itself. CI runners have no FUSE, so the
# build runs it with APPIMAGE_EXTRACT_AND_RUN=1, which unpacks the tool
# instead of mounting it. That requires squashfs-tools (mksquashfs) on the host.
#
# Usage:
#   ./scripts/build_appimage.sh [VERSION] [BINARY]
#
# Defaults: VERSION is the version in cmd/gputop/main.go, BINARY is the
# extracted release binary under dist/. GPUTOP_APPIMAGETOOL may point at an
# already downloaded appimagetool to skip the download.
set -euo pipefail

REPO_ROOT="${GPUTOP_SOURCE_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
MAIN="${REPO_ROOT}/cmd/gputop/main.go"
SOURCE_VERSION="$(sed -nE 's/^var version = "([^"]+)"$/\\1/p' "${MAIN}")"
VERSION="${1:-${SOURCE_VERSION}}"

if [ -z "${VERSION}" ]; then
    printf 'appimage: cannot read the version from %s\n' "${MAIN}" >&2
    exit 1
fi

ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64) TOOL_ARCH=x86_64 ;;
    aarch64 | arm64) TOOL_ARCH=aarch64 ;;
    i686 | i386) TOOL_ARCH=i686 ;;
    armv7l) TOOL_ARCH=armhf ;;
    *) TOOL_ARCH="${ARCH}" ;;
esac

TOOL_TAG="1.9.1"
TOOL_SHA256_x86_64="ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0"
TOOL_URL="https://github.com/AppImage/appimagetool/releases/download/${TOOL_TAG}/appimagetool-${TOOL_ARCH}.AppImage"

BINARY="${2:-${REPO_ROOT}/dist/gputop-${VERSION}-linux-${ARCH}/gputop}"
if [ ! -x "${BINARY}" ]; then
    printf 'appimage: no binary at %s; run scripts/release.sh first\n' "${BINARY}" >&2
    exit 1
fi

OUT="${REPO_ROOT}/dist"
APPDIR="${OUT}/AppDir"
TOOL="${GPUTOP_APPIMAGETOOL:-${OUT}/appimagetool.AppImage}"
TARGET="${OUT}/gputop-${VERSION}-linux-${ARCH}.AppImage"

# --- fetch and verify appimagetool -----------------------------------------
if [ ! -x "${TOOL}" ]; then
    printf 'appimage: downloading appimagetool %s (%s)\n' "${TOOL_TAG}" "${TOOL_ARCH}"
    mkdir -p "${OUT}"
    curl -fsSL -o "${TOOL}.tmp" "${TOOL_URL}"
    mv "${TOOL}.tmp" "${TOOL}"
    chmod 0755 "${TOOL}"
fi

if [ "${TOOL_ARCH}" = "x86_64" ]; then
    actual="$(sha256sum "${TOOL}" | cut -d' ' -f1)"
    if [ "${actual}" != "${TOOL_SHA256_x86_64}" ]; then
        printf 'appimage: appimagetool checksum mismatch:\n  expected %s\n  got      %s\n' \
            "${TOOL_SHA256_x86_64}" "${actual}" >&2
        exit 1
    fi
    printf 'appimage: appimagetool checksum verified\n'
fi

# --- assemble the AppDir ----------------------------------------------------
rm -rf "${APPDIR}"
mkdir -p "${APPDIR}/usr/bin" \
    "${APPDIR}/usr/share/icons/hicolor/scalable/apps" \
    "${APPDIR}/usr/share/applications"

install -m 0755 "${BINARY}" "${APPDIR}/usr/bin/gputop"
install -m 0755 "${REPO_ROOT}/packaging/appimage/AppRun" "${APPDIR}/AppRun"
install -m 0644 "${REPO_ROOT}/packaging/appimage/gputop.desktop" "${APPDIR}/gputop.desktop"
install -m 0644 "${REPO_ROOT}/packaging/appimage/gputop.svg" "${APPDIR}/gputop.svg"
install -m 0644 "${REPO_ROOT}/packaging/appimage/gputop.svg" \
    "${APPDIR}/usr/share/icons/hicolor/scalable/apps/gputop.svg"
ln -sf gputop.svg "${APPDIR}/.DirIcon"

# The desktop file has to sit in the standard application directory as well,
# or a host without the AppImage mounted would not find a launcher.
install -m 0644 "${REPO_ROOT}/packaging/appimage/gputop.desktop" \
    "${APPDIR}/usr/share/applications/gputop.desktop"

# --- build ------------------------------------------------------------------
printf 'appimage: packaging %s\n' "${TARGET}"
rm -f "${TARGET}"
APPIMAGE_EXTRACT_AND_RUN=1 "${TOOL}" "${APPDIR}" "${TARGET}"

if [ ! -s "${TARGET}" ]; then
    printf 'appimage: %s was not produced\n' "${TARGET}" >&2
    exit 1
fi
chmod 0755 "${TARGET}"

# The AppImage must actually run: --version exercises the AppRun, the mount
# and the binary, which is the whole point of shipping one.
reported="$(APPIMAGE_EXTRACT_AND_RUN=1 "${TARGET}" --version 2>/dev/null || true)"
if ! printf '%s' "${reported}" | grep -Fq "gputop ${VERSION}"; then
    printf 'appimage: the built AppImage reports %s, expected "gputop %s"\n' \
        "${reported:-<nothing>}" "${VERSION}" >&2
    exit 1
fi

(cd "${OUT}" && sha256sum "$(basename "${TARGET}")" > "$(basename "${TARGET}").sha256")

printf 'appimage: %s\n' "${TARGET}"
printf 'appimage: %s\n' "${TARGET}.sha256"
