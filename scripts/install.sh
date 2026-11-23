#!/bin/sh
# Installs the jitso CLI from a GitHub release, on macOS or Linux.
#
#   sh install.sh [version]          the latest release, or a tag such as v0.1.0
#
# Environment:
#   JITSO_VERSION       the release to install (an argument wins); default latest
#   JITSO_INSTALL_DIR   where the binary goes; default $HOME/.local/bin
#   JITSO_DOWNLOAD_URL  base of the releases; default https://github.com/upex-galaxy/jitso/releases
#                       (an https:// or file:// URL, or an absolute folder with the same layout)
#   JITSO_OS, JITSO_ARCH  override the detected platform (darwin or linux, amd64 or arm64)
#   JITSO_NO_MODIFY_PATH=1  never edit a shell profile (only print how to add the folder to PATH)
#
# When the install folder is not on PATH, it adds one marked line to the
# profile of your shell ($SHELL): ~/.zshrc (zsh), ~/.bashrc and on macOS also
# ~/.bash_profile (bash), ~/.config/fish/conf.d/jitso.fish (fish), else
# ~/.profile. A profile that already has the line is left alone.
#
# It downloads the archive and checksums.txt, checks the SHA-256, and moves
# the binary into place. It never uses sudo and runs nothing it downloaded
# except the new jitso binary, to print its version.
set -eu

releases="https://github.com/upex-galaxy/jitso/releases"

say() { printf 'jitso install: %s\n' "$*"; }
die() { printf 'jitso install: %s\n' "$*" >&2; exit 1; }
usage() {
  cat <<'USAGE'
Usage: sh install.sh [version]
Installs jitso into $JITSO_INSTALL_DIR (default $HOME/.local/bin).
version is a release tag such as v0.1.0, or latest (the default).
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac
if [ "$#" -gt 1 ]; then usage >&2; exit 2; fi

version=${1:-${JITSO_VERSION:-latest}}
case "$version" in
  latest) ;;
  v[0-9]*.[0-9]*.[0-9]*) ;;
  [0-9]*.[0-9]*.[0-9]*) version="v$version" ;;
  *) printf 'jitso install: not a version: %s (use a tag such as v0.1.0, or latest)\n' "$version" >&2; exit 2 ;;
esac
case "$version" in
  *[!A-Za-z0-9.+-]*) printf 'jitso install: bad characters in the version: %s\n' "$version" >&2; exit 2 ;;
esac

os=${JITSO_OS:-$(uname -s)}
case "$os" in
  Darwin|darwin) os=darwin ;;
  Linux|linux) os=linux ;;
  MINGW*|MSYS*|CYGWIN*|Windows_NT|windows)
    die "Windows is not supported by this script. In PowerShell, download install.ps1 from $releases (latest/download/install.ps1), read it, and run it: .\\install.ps1. jitso launch needs /bin/sh and does not run on Windows." ;;
  *) die "unsupported operating system: $os (supported: darwin, linux)" ;;
esac
arch=${JITSO_ARCH:-$(uname -m)}
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch (supported: amd64, arm64)" ;;
esac
# A shell under Rosetta reports x86_64 on Apple silicon: install the native build.
if [ -z "${JITSO_ARCH:-}" ] && [ "$os" = darwin ] && [ "$arch" = amd64 ] &&
  [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
  arch=arm64
fi

if [ -n "${JITSO_INSTALL_DIR:-}" ]; then
  dir=$JITSO_INSTALL_DIR
elif [ -n "${HOME:-}" ]; then
  dir="$HOME/.local/bin"
else
  die "HOME is not set; set JITSO_INSTALL_DIR to the folder for the binary"
fi
target="$dir/jitso"

# tilde is a path written ~/... when it lies under HOME.
tilde() {
  case "$1" in
    "${HOME:-/nonexistent}"/*) printf '~/%s' "${1#"$HOME"/}" ;;
    *) printf '%s' "$1" ;;
  esac
}

# ensure_path adds the install folder to PATH in the shell profile, once, when
# it is not on PATH already (see the header). JITSO_NO_MODIFY_PATH=1 only says how.
ensure_path() {
  case ":${PATH:-}:" in
    *":$dir:"*) return 0 ;;
  esac
  hint="export PATH=\"$dir:\$PATH\""
  if [ "${JITSO_NO_MODIFY_PATH:-}" = 1 ] || [ -z "${HOME:-}" ]; then
    say "$dir is not on your PATH; add it, for example: $hint"
    say "the hook that jitso setup hook registers runs the word jitso, so it needs jitso on PATH"
    return 0
  fi
  case "$dir" in
    "$HOME"/*) shown="\$HOME/${dir#"$HOME"/}" ;;
    *) shown=$dir ;;
  esac
  pline="export PATH=\"$shown:\$PATH\""
  case "$(basename "${SHELL:-sh}")" in
    zsh) files="${ZDOTDIR:-$HOME}/.zshrc" ;;
    bash)
      files="$HOME/.bashrc"
      [ "$os" = darwin ] && files="$files $HOME/.bash_profile" ;;
    fish)
      files="$HOME/.config/fish/conf.d/jitso.fish"
      pline="contains -- \"$dir\" \$PATH; or set -gx PATH \"$dir\" \$PATH" ;;
    *) files="$HOME/.profile" ;;
  esac
  marker="# added by the jitso installer"
  for f in $files; do
    if [ -f "$f" ] && grep -qF "$marker" "$f"; then
      continue
    fi
    mkdir -p "$(dirname "$f")" || { say "cannot write $(tilde "$f"); add it yourself: $hint"; continue; }
    if [ -s "$f" ]; then
      printf '\n%s\n%s\n' "$marker" "$pline" >>"$f" || { say "cannot write $(tilde "$f"); add it yourself: $hint"; continue; }
    else
      printf '%s\n%s\n' "$marker" "$pline" >>"$f" || { say "cannot write $(tilde "$f"); add it yourself: $hint"; continue; }
    fi
    say "added $dir to your PATH in $(tilde "$f")"
  done
  say "open a new terminal to use jitso, or run now: $hint"
}

if [ "$version" != latest ] && [ -x "$target" ]; then
  current=$("$target" version 2>/dev/null || true)
  case "$current" in
    "jitso $version "*) say "jitso $version is already installed at $target"; ensure_path; exit 0 ;;
  esac
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  die "need sha256sum or shasum to check the download; nothing installed"
fi

base=${JITSO_DOWNLOAD_URL:-$releases}
base=${base%/}
if [ "$version" = latest ]; then
  from="$base/latest/download"
else
  from="$base/download/$version"
fi

fetch() { # fetch <asset> <destination>
  case "$base" in
    /*)
      cp "$from/$1" "$2" 2>/dev/null || die "cannot read $from/$1" ;;
    https://*|file://*)
      if command -v curl >/dev/null 2>&1; then
        curl -fsSL --proto '=https,file' --proto-redir '=https' -o "$2" "$from/$1" ||
          die "download failed: $from/$1"
      elif command -v wget >/dev/null 2>&1; then
        case "$base" in file://*) die "a file:// URL needs curl" ;; esac
        wget -q -O "$2" "$from/$1" || die "download failed: $from/$1"
      else
        die "need curl or wget to download"
      fi ;;
    *) die "JITSO_DOWNLOAD_URL must be an https:// or file:// URL or an absolute folder: $base" ;;
  esac
}

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t jitso)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

asset="jitso-$os-$arch.tar.gz"
fetch checksums.txt "$tmp/checksums.txt"
fetch "$asset" "$tmp/$asset"

want=$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/checksums.txt")
case "$want" in
  "") die "checksums.txt has no line for $asset; nothing installed" ;;
  *[!0-9a-f]*) die "checksums.txt has a malformed line for $asset; nothing installed" ;;
esac
[ "${#want}" -eq 64 ] || die "checksums.txt has a malformed line for $asset; nothing installed"
got=$(sha256 "$tmp/$asset")
[ "$got" = "$want" ] || die "checksum mismatch for $asset (expected $want, got $got); nothing installed"

mkdir "$tmp/x"
tar -xzf "$tmp/$asset" -C "$tmp/x" || die "cannot extract $asset; nothing installed"
new="$tmp/x/jitso-$os-$arch/jitso"
[ -f "$new" ] || die "$asset has no jitso-$os-$arch/jitso; nothing installed"
chmod 0755 "$new"
line=$("$new" version 2>/dev/null) || die "the downloaded jitso does not run on this machine; nothing installed"

old=""
if [ -x "$target" ]; then
  old=$("$target" version 2>/dev/null || true)
  if [ "$old" = "$line" ]; then
    say "$line is already installed at $target"
    ensure_path
    exit 0
  fi
fi

mkdir -p "$dir" || die "cannot create $dir; set JITSO_INSTALL_DIR to a folder you own"
cp "$new" "$dir/.jitso.tmp.$$" || die "cannot write to $dir; set JITSO_INSTALL_DIR to a folder you own"
chmod 0755 "$dir/.jitso.tmp.$$"
mv -f "$dir/.jitso.tmp.$$" "$target" || { rm -f "$dir/.jitso.tmp.$$"; die "cannot replace $target"; }

if [ -n "$old" ]; then
  say "upgraded $target: $old -> $line"
else
  say "installed $line at $target"
fi
ensure_path
say "next: run jitso init in your project to set it up (skill, AGENTS.md block, guard)"
