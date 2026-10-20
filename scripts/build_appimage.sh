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
SOURCE_VERSION="$(sed -nE 's/^var version = "([^"]+)"$/\1/p' "${MAIN}")"
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

# appimagetool embeds a type2 runtime and, left to itself, downloads the
# "continuous" build of it. That makes the AppImage different on every run
# even from an identical AppDir, so the runtime is fetched from a pinned
# release instead and passed in with --runtime-file.
RUNTIME_TAG="20251108"
RUNTIME_URL="https://github.com/AppImage/type2-runtime/releases/download/${RUNTIME_TAG}/runtime-${TOOL_ARCH}"
RUNTIME_SHA256_x86_64="2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d"
RUNTIME_SHA256_aarch64="00cbdfcf917cc6c0ff6d3347d59e0ca1f7f45a6df1a428a0d6d8a78664d87444"
RUNTIME_SHA256_armhf="e9060d37577b8a29914ec12d8740add24e19ff29012fb1fa0f60daf62db0688d"
RUNTIME_SHA256_i686="e72ea0b140a0a16e680713238a6f30aad278b62c4ca17919c554864124515498"

BINARY="${2:-${REPO_ROOT}/dist/gputop-${VERSION}-linux-${ARCH}/gputop}"
if [ ! -x "${BINARY}" ]; then
    printf 'appimage: no binary at %s; run scripts/release.sh first\n' "${BINARY}" >&2
    exit 1
fi

OUT="${REPO_ROOT}/dist"
APPDIR="${OUT}/AppDir"
# The tool lives in dist/tools/ rather than dist/ itself: the release workflow
# uploads dist/*.AppImage as an asset, and a tool named *.AppImage in dist/
# would be shipped to users as if it were gputop.
TOOLDIR="${OUT}/tools"
TOOL="${GPUTOP_APPIMAGETOOL:-${TOOLDIR}/appimagetool.AppImage}"
TARGET="${OUT}/gputop-${VERSION}-linux-${ARCH}.AppImage"

# --- fetch and verify appimagetool -----------------------------------------
if [ ! -x "${TOOL}" ]; then
    printf 'appimage: downloading appimagetool %s (%s)\n' "${TOOL_TAG}" "${TOOL_ARCH}"
    mkdir -p "${TOOLDIR}"
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

# --- fetch and verify the type2 runtime -------------------------------------
RUNTIME="${TOOLDIR}/runtime-${TOOL_ARCH}"
runtime_sha_var="RUNTIME_SHA256_${TOOL_ARCH}"
runtime_sha="${!runtime_sha_var:-}"
if [ ! -s "${RUNTIME}" ]; then
    printf 'appimage: downloading the type2 runtime %s (%s)\n' "${RUNTIME_TAG}" "${TOOL_ARCH}"
    curl -fsSL -o "${RUNTIME}.tmp" "${RUNTIME_URL}"
    mv "${RUNTIME}.tmp" "${RUNTIME}"
    chmod 0755 "${RUNTIME}"
fi
if [ -n "${runtime_sha}" ]; then
    actual="$(sha256sum "${RUNTIME}" | cut -d' ' -f1)"
    if [ "${actual}" != "${runtime_sha}" ]; then
        printf 'appimage: type2 runtime checksum mismatch:\n  expected %s\n  got      %s\n' \
            "${runtime_sha}" "${actual}" >&2
        exit 1
    fi
    printf 'appimage: type2 runtime checksum verified\n'
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
# mkdir and install carry the umask into the AppDir and squashfs records the
# modes it finds, so a directory created under umask 0002 would be 0775 where
# the same build under umask 0022 gives 0755, and the AppImage would differ.
# The modes are pinned for the same reason the timestamps are pinned below: the
# artifact has to be a function of the source, not of the machine.
find "${APPDIR}" -type d -exec chmod 0755 {} +
find "${APPDIR}" -type f -exec chmod 0644 {} +
chmod 0755 "${APPDIR}/AppRun" "${APPDIR}/usr/bin/gputop"

printf 'appimage: packaging %s\n' "${TARGET}"
rm -f "${TARGET}"
# mksquashfs records the modification time of every file, so an AppDir whose
# files carry the time of the build yields a different AppImage on every run.
# -all-time pins them all to the commit date, which makes the artifact a
# function of the source rather than of the moment it was built.
#
# SOURCE_DATE_EPOCH is unset for this call on purpose: mksquashfs refuses to
# combine it with -all-time ("can't be used at the same time to set
# timestamp(s)"), and a caller who exports that variable - the reproducible
# build convention, and what release.sh reads - would otherwise turn a correct
# build into a hard failure.
epoch="${SOURCE_DATE_EPOCH:-$(git -C "${REPO_ROOT}" log -1 --format=%ct 2>/dev/null || date +%s)}"
env -u SOURCE_DATE_EPOCH APPIMAGE_EXTRACT_AND_RUN=1 "${TOOL}" --runtime-file "${RUNTIME}" \
    --mksquashfs-opt "-all-time" --mksquashfs-opt "${epoch}" \
    "${APPDIR}" "${TARGET}"

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
