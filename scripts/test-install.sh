#!/bin/sh
# Tests scripts/install.sh without a network, against archives built by
# scripts/package.sh for the platform this script runs on.
#
# Usage, from the repo root:
#   scripts/test-install.sh                    builds the host target itself
#   scripts/test-install.sh <dist dir> <tag>   uses archives already built by
#                                              package.sh (the release workflow)
#
# Every check prints "ok <name>" or "FAIL <name>"; the exit code is 1 when any
# check fails. It runs in throwaway folders under mktemp -d and never touches
# the real HOME. JITSO_TEST_SH names the shell that runs install.sh (default sh).
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
install_sh="$root/scripts/install.sh"
sh_bin=${JITSO_TEST_SH:-sh}

case $# in
  0) dist=""; tag=v0.0.0-test ;;
  2) dist=$1; tag=$2 ;;
  *) echo "usage: scripts/test-install.sh [<dist dir> <tag>]" >&2; exit 2 ;;
esac
case "$tag" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "test-install: the tag must look like v1.2.3: $tag" >&2; exit 2 ;;
esac
tag2=v0.0.1-test
[ "$tag" != "$tag2" ] || tag2=v0.0.2-test

# The install script reads these; a stray value from the caller must not leak in.
unset JITSO_VERSION JITSO_INSTALL_DIR JITSO_DOWNLOAD_URL JITSO_OS JITSO_ARCH

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t jitso-test)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

host_os=$(go env GOOS)
host_arch=$(go env GOARCH)
asset="jitso-$host_os-$host_arch.tar.gz"
case "$host_os" in
  darwin|linux) ;;
  *) echo "test-install: the install script supports darwin and linux, not $host_os" >&2; exit 1 ;;
esac

# Layout of a GitHub releases folder: download/<tag>/<assets> and
# latest/download/<assets>.
rel="$tmp/releases"
mkdir -p "$rel/download/$tag" "$rel/latest/download"
if [ -n "$dist" ]; then
  [ -f "$dist/checksums.txt" ] && [ -f "$dist/$asset" ] ||
    { echo "test-install: $dist has no checksums.txt or $asset" >&2; exit 1; }
  cp "$dist"/* "$rel/download/$tag/"
else
  sh "$root/scripts/package.sh" "$tag" "$rel/download/$tag" "$host_os/$host_arch" >/dev/null
fi
cp "$rel/download/$tag"/* "$rel/latest/download/"

failed=0
pass() { printf 'ok   %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2"; failed=1; }

out="$tmp/out"
err="$tmp/err"
rc=0
# run VAR=value ... command args: runs with the given variables, keeps the exit
# code in $rc and the output in $out and $err.
run() {
  set +e
  env "$@" >"$out" 2>"$err"
  rc=$?
  set -e
}
has() { grep -F -q -- "$2" "$1"; } # has <file> <text>
newhome() { # newhome <name>: an empty HOME that is not on the PATH
  mkdir -p "$tmp/$1"
  printf '%s' "$tmp/$1"
}

# 1 pinned install
h1=$(newhome home1)
run HOME="$h1" JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh" "$tag"
bin="$h1/.local/bin/jitso"
if [ "$rc" -eq 0 ] && [ -x "$bin" ] && "$bin" version | grep -q "^jitso $tag " && has "$out" "is not on your PATH"; then
  pass "pinned install"
else
  fail "pinned install" "rc=$rc $(cat "$err")"
fi

# 2 already installed: nothing is rewritten
touch -t 200001010000 "$bin"
before=$(ls -l "$bin")
run HOME="$h1" JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh" "$tag"
after=$(ls -l "$bin")
if [ "$rc" -eq 0 ] && has "$out" "is already installed" && [ "$before" = "$after" ]; then
  pass "already installed"
else
  fail "already installed" "rc=$rc"
fi

# 3 latest
run HOME="$h1" JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh"
if [ "$rc" -eq 0 ] && has "$out" "is already installed"; then
  pass "latest"
else
  fail "latest" "rc=$rc $(cat "$err")"
fi

# 3b the version can come from the environment, and a missing v is added
h1b=$(newhome home1b)
run HOME="$h1b" JITSO_DOWNLOAD_URL="$rel" JITSO_VERSION="${tag#v}" "$sh_bin" "$install_sh"
if [ "$rc" -eq 0 ] && "$h1b/.local/bin/jitso" version | grep -q "^jitso $tag "; then
  pass "version from the environment"
else
  fail "version from the environment" "rc=$rc $(cat "$err")"
fi

# 4 file:// URL (needs curl)
if command -v curl >/dev/null 2>&1; then
  h4=$(newhome home4)
  run HOME="$h4" JITSO_DOWNLOAD_URL="file://$rel" "$sh_bin" "$install_sh" "$tag"
  if [ "$rc" -eq 0 ] && [ -x "$h4/.local/bin/jitso" ]; then
    pass "file:// URL"
  else
    fail "file:// URL" "rc=$rc $(cat "$err")"
  fi
else
  echo "skip file:// URL (no curl)"
fi

# 5 corrupted archive
bad="$tmp/bad"
mkdir -p "$bad"
cp -R "$rel/." "$bad/"
printf 'x' >> "$bad/download/$tag/$asset"
h5=$(newhome home5)
run HOME="$h5" JITSO_DOWNLOAD_URL="$bad" "$sh_bin" "$install_sh" "$tag"
if [ "$rc" -eq 1 ] && has "$err" "checksum mismatch" && [ ! -e "$h5/.local/bin/jitso" ]; then
  pass "corrupted archive is rejected"
else
  fail "corrupted archive is rejected" "rc=$rc $(cat "$err")"
fi

# 6 wrong checksum
wrong="$tmp/wrong"
mkdir -p "$wrong"
cp -R "$rel/." "$wrong/"
zeros=0000000000000000000000000000000000000000000000000000000000000000
awk -v f="$asset" -v z="$zeros" '$2 == f { $1 = z } { print }' "$rel/download/$tag/checksums.txt" > "$wrong/download/$tag/checksums.txt"
h6=$(newhome home6)
run HOME="$h6" JITSO_DOWNLOAD_URL="$wrong" "$sh_bin" "$install_sh" "$tag"
if [ "$rc" -eq 1 ] && has "$err" "checksum mismatch" && [ ! -e "$h6/.local/bin/jitso" ]; then
  pass "wrong checksum is rejected"
else
  fail "wrong checksum is rejected" "rc=$rc $(cat "$err")"
fi

# 7 missing checksum line
gone="$tmp/gone"
mkdir -p "$gone"
cp -R "$rel/." "$gone/"
awk -v f="$asset" '$2 != f' "$rel/download/$tag/checksums.txt" > "$gone/download/$tag/checksums.txt"
h7=$(newhome home7)
run HOME="$h7" JITSO_DOWNLOAD_URL="$gone" "$sh_bin" "$install_sh" "$tag"
if [ "$rc" -eq 1 ] && has "$err" "has no line for" && [ ! -e "$h7/.local/bin/jitso" ]; then
  pass "missing checksum line is rejected"
else
  fail "missing checksum line is rejected" "rc=$rc $(cat "$err")"
fi

# 7b malformed checksum line
mal="$tmp/mal"
mkdir -p "$mal"
cp -R "$rel/." "$mal/"
awk -v f="$asset" '$2 == f { $1 = "abc" } { print }' "$rel/download/$tag/checksums.txt" > "$mal/download/$tag/checksums.txt"
h7b=$(newhome home7b)
run HOME="$h7b" JITSO_DOWNLOAD_URL="$mal" "$sh_bin" "$install_sh" "$tag"
if [ "$rc" -eq 1 ] && has "$err" "malformed line" && [ ! -e "$h7b/.local/bin/jitso" ]; then
  pass "malformed checksum line is rejected"
else
  fail "malformed checksum line is rejected" "rc=$rc $(cat "$err")"
fi

# 8 neither sha tool: fails closed. The PATH holds symlinks to the tools the
# script may use before it checks, and no SHA-256 tool.
nosha="$tmp/nosha"
mkdir -p "$nosha"
shpath=$(command -v "$sh_bin" 2>/dev/null || true)
[ -n "$shpath" ] || shpath=$sh_bin
for tool in uname tar mktemp cp mv chmod mkdir rm awk cat sysctl gzip dirname; do
  p=$(command -v "$tool" 2>/dev/null || true)
  case "$p" in
    /*) ln -s "$p" "$nosha/$tool" ;;
  esac
done
h8=$(newhome home8)
run HOME="$h8" PATH="$nosha" JITSO_DOWNLOAD_URL="$rel" "$shpath" "$install_sh" "$tag"
if [ "$rc" -eq 1 ] && has "$err" "need sha256sum or shasum" && [ ! -e "$h8/.local/bin/jitso" ]; then
  pass "no sha tool fails closed"
else
  fail "no sha tool fails closed" "rc=$rc $(cat "$err")"
fi

# 9 Windows is refused
run HOME="$h8" JITSO_OS=windows JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh" "$tag"
if [ "$rc" -eq 1 ] && has "$err" "Windows is not supported by this script"; then
  pass "Windows is refused"
else
  fail "Windows is refused" "rc=$rc $(cat "$err")"
fi

# 9b another operating system and another architecture
run HOME="$h8" JITSO_OS=plan9 JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh" "$tag"
if [ "$rc" -eq 1 ] && has "$err" "unsupported operating system: plan9"; then
  pass "unsupported operating system"
else
  fail "unsupported operating system" "rc=$rc $(cat "$err")"
fi
run HOME="$h8" JITSO_ARCH=riscv64 JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh" "$tag"
if [ "$rc" -eq 1 ] && has "$err" "unsupported architecture: riscv64"; then
  pass "unsupported architecture"
else
  fail "unsupported architecture" "rc=$rc $(cat "$err")"
fi

# 10 bad version
run HOME="$h8" JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh" 'v1.0.0;rm'
if [ "$rc" -eq 2 ]; then
  pass "bad version is a usage error"
else
  fail "bad version is a usage error" "rc=$rc"
fi
run HOME="$h8" JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh" one two
if [ "$rc" -eq 2 ]; then
  pass "too many arguments is a usage error"
else
  fail "too many arguments is a usage error" "rc=$rc"
fi

# 11 upgrade: a second version into the same layout, installed over the first
mkdir -p "$rel/download/$tag2"
sh "$root/scripts/package.sh" "$tag2" "$rel/download/$tag2" "$host_os/$host_arch" >/dev/null
run HOME="$h1" JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh" "$tag2"
if [ "$rc" -eq 0 ] && has "$out" "upgraded" && "$bin" version | grep -q "^jitso $tag2 "; then
  pass "upgrade"
else
  fail "upgrade" "rc=$rc $(cat "$err")"
fi
# version pinning: the pinned tag is exactly the one installed, and an older
# tag installs over a newer one
run HOME="$h1" JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh" "$tag"
if [ "$rc" -eq 0 ] && "$bin" version | grep -q "^jitso $tag "; then
  pass "version pinning"
else
  fail "version pinning" "rc=$rc $(cat "$err")"
fi

# 12 install dir override
h12=$(newhome home12)
run HOME="$h12" JITSO_INSTALL_DIR="$tmp/bin2" JITSO_DOWNLOAD_URL="$rel" "$sh_bin" "$install_sh" "$tag"
if [ "$rc" -eq 0 ] && [ -x "$tmp/bin2/jitso" ] && [ ! -e "$h12/.local" ]; then
  pass "install dir override"
else
  fail "install dir override" "rc=$rc $(cat "$err")"
fi

# 13 no temp file is left in the install dir
if [ -z "$(ls -A "$tmp/bin2" | grep -v '^jitso$' || true)" ]; then
  pass "install dir holds only jitso"
else
  fail "install dir holds only jitso" "$(ls -A "$tmp/bin2")"
fi

if [ "$failed" -ne 0 ]; then
  echo "test-install: FAILED" >&2
  exit 1
fi
echo "test-install: all checks passed"
