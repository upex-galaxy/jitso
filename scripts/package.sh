#!/bin/sh
# Builds the release archives and checksums.txt. This script is the one
# definition of the asset names; the release workflow and
# scripts/test-install.sh both call it.
#
# Usage: scripts/package.sh <version> <out dir> [<os>/<arch> ...]
#
# With no target it builds darwin, linux and windows for amd64 and arm64.
# Archives hold one folder jitso-<os>-<arch>/ with the binary, LICENSE and
# README.md: jitso-<os>-<arch>.tar.gz (darwin, linux), jitso-<os>-<arch>.zip
# (windows). checksums.txt has one line per asset, in the format of
# sha256sum, and covers the archives, install.sh and install.ps1.
set -eu

usage="usage: scripts/package.sh <version> <out dir> [<os>/<arch> ...]"
[ "$#" -ge 2 ] || { echo "$usage" >&2; exit 2; }
version=$1
out=$2
shift 2
case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "package: the version must look like v1.2.3: $version" >&2; exit 2 ;;
esac
case "$version" in
  *[!A-Za-z0-9.+-]*) echo "package: bad characters in $version" >&2; exit 2 ;;
esac
targets=${*:-"darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64"}
for t in $targets; do
  case "$t" in
    */*) ;;
    *) echo "package: a target looks like <os>/<arch>: $t" >&2; exit 2 ;;
  esac
  case "$t" in
    *[!a-z0-9/]*) echo "package: bad characters in the target: $t" >&2; exit 2 ;;
  esac
done
root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)
stage="$out/.stage"
rm -rf "$stage"
mkdir -p "$stage"
# Keep macOS from adding AppleDouble entries to the tar archives.
COPYFILE_DISABLE=1
export COPYFILE_DISABLE
for t in $targets; do
  os=${t%/*}
  arch=${t#*/}
  name="jitso-$os-$arch"
  exe=jitso
  [ "$os" = windows ] && exe=jitso.exe
  mkdir -p "$stage/$name"
  (cd "$root" && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X github.com/upex-galaxy/jitso/internal/version.Version=$version" \
    -o "$stage/$name/$exe" ./cmd/jitso)
  cp "$root/LICENSE" "$root/README.md" "$stage/$name/"
  if [ "$os" = windows ]; then
    command -v zip >/dev/null 2>&1 || { echo "package: need zip for the windows archives" >&2; exit 1; }
    rm -f "$out/$name.zip"
    (cd "$stage" && zip -qr "$out/$name.zip" "$name")
  else
    rm -f "$out/$name.tar.gz"
    tar -C "$stage" -czf "$out/$name.tar.gz" "$name"
  fi
done
cp "$root/scripts/install.sh" "$root/scripts/install.ps1" "$out/"
rm -rf "$stage"
cd "$out"
# Only the assets of this run: the archives of the requested targets, then the install scripts.
set --
for t in $targets; do
  os=${t%/*}
  arch=${t#*/}
  if [ "$os" = windows ]; then
    set -- "$@" "jitso-$os-$arch.zip"
  else
    set -- "$@" "jitso-$os-$arch.tar.gz"
  fi
done
set -- "$@" install.sh install.ps1
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$@" > checksums.txt
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 "$@" > checksums.txt
else
  echo "package: need sha256sum or shasum" >&2
  exit 1
fi
echo "package: wrote $# files and checksums.txt to $out"
