#!/bin/sh
# Checks a published release for the platform it runs on (docs/release.md,
# section 4).
#
#   scripts/verify-release.sh <tag>      a published tag, such as v0.1.0
#
# With gh it downloads checksums.txt, the archive of this OS and architecture and
# install.sh of the tag, from the repository of the origin remote (default
# upex-galaxy/jitso). It checks the SHA-256 of the archive and of install.sh,
# runs the jitso of the archive, runs the downloaded install.sh, and runs the
# jitso that install.sh installed; both version lines have to name the tag.
#
# Everything happens in a temp folder that is deleted on exit. What it downloaded
# runs with an empty environment, a HOME inside that folder and
# JITSO_NO_MODIFY_PATH=1, so no variable of the caller (HOME, ZDOTDIR, JITSO_*)
# can point it at the real HOME or at a shell profile.
#
# Exit 0 when the release holds, 1 with one line on stderr when it does not, 2
# for a bad invocation. macOS and Linux only.
set -eu

say() { printf 'verify-release: %s\n' "$*"; }
die() { printf 'verify-release: %s\n' "$*" >&2; exit 1; }
usage() { printf 'usage: scripts/verify-release.sh <tag>, a tag such as v0.1.0\n' >&2; exit 2; }
last() { tail -n 1 "$1"; } # the last line a failed command wrote

[ "$#" -eq 1 ] || usage
tag=$1
case "$tag" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) usage ;;
esac
case "$tag" in
  *[!A-Za-z0-9.+-]*) usage ;;
esac

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die "unsupported operating system: $(uname -s) (darwin and linux only)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m) (amd64 and arm64 only)" ;;
esac
# A shell under Rosetta reports x86_64 on Apple silicon, as install.sh knows.
if [ "$os" = darwin ] && [ "$arch" = amd64 ] &&
  [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
  arch=arm64
fi

command -v gh >/dev/null 2>&1 || die "need gh, the GitHub CLI, to download the release"
if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  die "need sha256sum or shasum to check the download"
fi

root=$(cd "$(dirname "$0")/.." && pwd)
url=$(git -C "$root" remote get-url origin 2>/dev/null || true)
repo=""
case "$url" in
  https://github.com/*) repo=${url#https://github.com/} ;;
  git@github.com:*) repo=${url#git@github.com:} ;;
  ssh://git@github.com/*) repo=${url#ssh://git@github.com/} ;;
esac
repo=${repo%/}
repo=${repo%.git}
[ -n "$repo" ] || repo=upex-galaxy/jitso

tmp=$(mktemp -d "${TMPDIR:-/tmp}/jitso-verify.XXXXXX") || die "cannot create a temp folder"
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM
mkdir "$tmp/download" "$tmp/extract" "$tmp/home"

asset="jitso-$os-$arch.tar.gz"
GH_NO_UPDATE_NOTIFIER=1 gh release download "$tag" -R "$repo" -D "$tmp/download" \
  -p checksums.txt -p "$asset" -p install.sh >/dev/null 2>"$tmp/err" ||
  die "cannot download $tag from $repo: $(last "$tmp/err")"
for f in checksums.txt "$asset" install.sh; do
  [ -f "$tmp/download/$f" ] || die "release $tag of $repo has no $f"
done

for f in "$asset" install.sh; do
  want=$(awk -v f="$f" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/download/checksums.txt")
  [ -n "$want" ] || die "checksums.txt has no line for $f"
  got=$(sha256 "$tmp/download/$f")
  [ "$got" = "$want" ] || die "checksum mismatch for $f (expected $want, got $got)"
  say "ok checksum of $f"
done

# isolated <command> ...: runs it with nothing of the caller's environment but PATH.
isolated() {
  env -i PATH="$PATH" HOME="$tmp/home" TMPDIR="$tmp" SHELL=/bin/sh \
    JITSO_NO_MODIFY_PATH=1 JITSO_DOWNLOAD_URL="https://github.com/$repo/releases" "$@"
}

# check_version <binary> <what>: its version line has to name the tag.
check_version() {
  line=$(isolated "$1" version 2>"$tmp/err") || die "$2 does not run: $(last "$tmp/err")"
  line=$(printf '%s\n' "$line" | head -n 1)
  case "$line" in
    "jitso $tag "*) say "ok $2: $line" ;;
    *) die "$2 reports \"$line\", not $tag" ;;
  esac
}

tar -xzf "$tmp/download/$asset" -C "$tmp/extract" 2>"$tmp/err" ||
  die "cannot extract $asset: $(last "$tmp/err")"
bin="$tmp/extract/jitso-$os-$arch/jitso"
[ -f "$bin" ] && [ -x "$bin" ] || die "$asset has no executable jitso-$os-$arch/jitso"
check_version "$bin" "the jitso of $asset"

isolated sh "$tmp/download/install.sh" "$tag" >/dev/null 2>"$tmp/err" ||
  die "the downloaded install.sh failed: $(last "$tmp/err")"
installed="$tmp/home/.local/bin/jitso"
[ -x "$installed" ] || die "the downloaded install.sh left no jitso in its HOME"
check_version "$installed" "the installed jitso"

say "$tag of $repo holds on $os/$arch"
