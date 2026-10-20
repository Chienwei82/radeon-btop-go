#!/usr/bin/env bash
# check_packaging.sh — keep the shipped documentation honest.
#
# The man page and the three shell completions are hand-written, so they can
# drift from the CLI without any Go test noticing. This script reads the flag
# names and the version out of the source of truth (cmd/gputop/main.go,
# internal/model/theme.go, Containerfile) and fails when a shipped file is
# missing an option, names a version that no longer exists, or still carries a
# Phase 0 placeholder. It runs on the host: bash, grep and sed only.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MAIN="${REPO_ROOT}/cmd/gputop/main.go"
THEMES_GO="${REPO_ROOT}/internal/model/theme.go"
MAN="${REPO_ROOT}/packaging/man/gputop.1"
BASH_COMP="${REPO_ROOT}/packaging/completions/gputop.bash"
ZSH_COMP="${REPO_ROOT}/packaging/completions/gputop.zsh"
FISH_COMP="${REPO_ROOT}/packaging/completions/gputop.fish"
PKGBUILD="${REPO_ROOT}/packaging/arch/PKGBUILD"
CONTAINERFILE="${REPO_ROOT}/Containerfile"
APPDESKTOP="${REPO_ROOT}/packaging/appimage/gputop.desktop"
APPICON="${REPO_ROOT}/packaging/appimage/gputop.svg"
APPRUN="${REPO_ROOT}/packaging/appimage/AppRun"

fail=0
problems=0

problem() {
    printf 'packaging: %s\n' "$*" >&2
    problems=$((problems + 1))
    fail=1
}

# Flags registered with flag.Bool/String/Float64 and their *Var forms. The
# short aliases (-c, -i) are registered as separate names on purpose.
flags="$( \
    { grep -oE 'fs\.(Bool|String|Float64)\("[a-z-]+"' "${MAIN}" \
        | sed -E 's/.*\("//; s/"$//'
      grep -oE 'fs\.(Bool|String|Float64)Var\([^,]+, "[a-z-]+"' "${MAIN}" \
        | sed -E 's/.*, "//; s/"$//'
    } | sort -u)"
if [ -z "${flags}" ]; then
    problem "no flags found in ${MAIN}"
fi

for flag in ${flags}; do
    if [ "${#flag}" -eq 1 ]; then
        # A short alias is written as \-c in roff, not \-\-c.
        grep -Fq -- "\\-${flag}\\fR" "${MAN}" || problem "man page misses -${flag}"
        grep -Fq -- "-${flag} " "${BASH_COMP}" || problem "bash completion misses -${flag}"
        grep -Fq -- "{-${flag}," "${ZSH_COMP}" || problem "zsh completion misses -${flag}"
        grep -Fq -- "-s ${flag} " "${FISH_COMP}" || problem "fish completion misses -${flag}"
    else
        grep -Fq -- "\\-\\-${flag}" "${MAN}" || problem "man page misses --${flag}"
        grep -Fq -- "--${flag}" "${BASH_COMP}" || problem "bash completion misses --${flag}"
        grep -Fq -- "--${flag}" "${ZSH_COMP}" || problem "zsh completion misses --${flag}"
        grep -Fq -- "-l ${flag}" "${FISH_COMP}" || problem "fish completion misses --${flag}"
    fi
done

# Theme names are part of the model, and two flags offer them.
themes="$(sed -nE 's/^var ThemeNames = \[\]string\{(.*)\}$/\1/p' "${THEMES_GO}" \
    | tr -d '"' | tr ',' '\n' | tr -d ' ' | sed '/^$/d')"
if [ -z "${themes}" ]; then
    problem "no theme names found in ${THEMES_GO}"
fi
for theme in ${themes}; do
    grep -Fq -- "${theme}" "${MAN}" || problem "man page misses theme ${theme}"
    grep -Fq -- "${theme}" "${BASH_COMP}" || problem "bash completion misses theme ${theme}"
    grep -Fq -- "${theme}" "${ZSH_COMP}" || problem "zsh completion misses theme ${theme}"
    grep -Fq -- "${theme}" "${FISH_COMP}" || problem "fish completion misses theme ${theme}"
done

# One version, four places.
source_version="$(sed -nE 's/^var version = "([^"]+)"$/\1/p' "${MAIN}")"
man_version="$(sed -nE 's/^\.TH GPUTOP 1 "[^"]*" "gputop ([^"]+)".*/\1/p' "${MAN}")"
package_version="$(sed -nE 's/^pkgver=(.*)$/\1/p' "${PKGBUILD}")"
image_version="$(sed -nE 's/^ARG VERSION=(.*)$/\1/p' "${CONTAINERFILE}" | head -1)"
if [ -z "${source_version}" ]; then
    problem "no version found in ${MAIN}"
fi
for pair in "man:${man_version}" "PKGBUILD:${package_version}" "Containerfile:${image_version}"; do
    name="${pair%%:*}"
    value="${pair#*:}"
    if [ "${value}" != "${source_version}" ]; then
        problem "version drift: ${name} says ${value:-<empty>}, source says ${source_version}"
    fi
done

# The AppImage is a third shipped artifact: its desktop file, icon and AppRun
# are hand-written too, so they can drift from the CLI the same way.
for f in "${APPDESKTOP}" "${APPICON}" "${APPRUN}"; do
    [ -f "${f}" ] || problem "missing AppImage source ${f}"
done
if [ -f "${APPDESKTOP}" ]; then
    grep -Fq "Exec=gputop" "${APPDESKTOP}" || problem "AppImage desktop file does not exec gputop"
    grep -Fq "Icon=gputop" "${APPDESKTOP}" || problem "AppImage desktop file names no icon"
    grep -Fq "Terminal=true" "${APPDESKTOP}" || problem "AppImage desktop file does not ask for a terminal"
fi
if [ -f "${APPRUN}" ]; then
    grep -Fq 'usr/bin/gputop' "${APPRUN}" || problem "AppRun does not exec the packaged binary"
fi

# Phase 0 files were placeholders; a release must not ship one.
if grep -Rqi 'placeholder' "${REPO_ROOT}/packaging/man" "${REPO_ROOT}/packaging/completions"; then
    problem "a shipped man page or completion still says 'placeholder'"
fi

# Syntax checks that need no toolchain. zsh and fish are optional on a
# release host, so their absence is reported, not treated as failure.
bash -n "${BASH_COMP}" || problem "bash completion does not parse"
bash -n "${PKGBUILD}" || problem "PKGBUILD does not parse"
if command -v zsh >/dev/null 2>&1; then
    zsh -n "${ZSH_COMP}" || problem "zsh completion does not parse"
else
    printf 'packaging: zsh not installed, syntax check skipped\n'
fi
if command -v fish >/dev/null 2>&1; then
    fish -n "${FISH_COMP}" || problem "fish completion does not parse"
else
    printf 'packaging: fish not installed, syntax check skipped\n'
fi
if command -v groff >/dev/null 2>&1; then
    groff -man -Tascii -ww "${MAN}" >/dev/null 2>/tmp/gputop-groff.$$ \
        || problem "man page does not render"
    if [ -s /tmp/gputop-groff.$$ ]; then
        sed 's/^/packaging: groff: /' /tmp/gputop-groff.$$ >&2
    fi
    rm -f /tmp/gputop-groff.$$
else
    printf 'packaging: groff not installed, man page check skipped\n'
fi

if [ "${fail}" -ne 0 ]; then
    printf 'packaging: %d problem(s)\n' "${problems}" >&2
    exit 1
fi
printf 'packaging: %d flags and %d themes documented consistently (version %s)\n' \
    "$(printf '%s\n' "${flags}" | wc -l)" "$(printf '%s\n' "${themes}" | wc -l)" "${source_version}"
