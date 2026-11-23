# Installs the jitso CLI from a GitHub release, on Windows (PowerShell 5.1 or 7).
#
#   .\install.ps1 [version]          the latest release, or a tag such as v0.1.0
#
# Environment:
#   JITSO_VERSION       the release to install (an argument wins); default latest
#   JITSO_INSTALL_DIR   where jitso.exe goes; default %LOCALAPPDATA%\Programs\jitso
#   JITSO_DOWNLOAD_URL  base of the releases; default https://github.com/upex-galaxy/jitso/releases
#                       (an https:// or file:// URL, or an absolute folder with the same layout)
#   JITSO_ARCH          override the detected architecture (amd64 or arm64)
#   JITSO_NO_MODIFY_PATH=1  never edit the user PATH (only print how to add the folder)
#
# When the install folder is not on the user PATH, it appends it to the user
# PATH of your account (HKCU\Environment), once. It downloads the archive and
# checksums.txt, checks the SHA-256, and moves jitso.exe into place. It needs no
# administrator rights and runs nothing it downloaded except the new jitso.exe,
# to print its version. jitso launch needs /bin/sh and does not run on Windows;
# every other command does.
[CmdletBinding()]
param([Parameter(Position = 0)][string]$Version)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$releases = 'https://github.com/upex-galaxy/jitso/releases'

function Say([string]$Message) { Write-Host "jitso install: $Message" }
function Die([string]$Message, [int]$Code = 1) {
  [Console]::Error.WriteLine("jitso install: $Message")
  exit $Code
}

if (-not $Version) { $Version = $env:JITSO_VERSION }
if (-not $Version) { $Version = 'latest' }
if ($Version -eq 'latest') {
} elseif ($Version -match '^v\d+\.\d+\.\d+') {
} elseif ($Version -match '^\d+\.\d+\.\d+') {
  $Version = "v$Version"
} else {
  Die "not a version: $Version (use a tag such as v0.1.0, or latest)" 2
}
if ($Version -notmatch '^[A-Za-z0-9.+-]+$') { Die "bad characters in the version: $Version" 2 }

$arch = $env:JITSO_ARCH
if (-not $arch) {
  $native = $null
  try { $native = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() } catch { }
  if (-not $native) {
    $native = $env:PROCESSOR_ARCHITEW6432
    if (-not $native) { $native = $env:PROCESSOR_ARCHITECTURE }
  }
  switch -Regex ($native) {
    '^(X64|AMD64)$' { $arch = 'amd64' }
    '^ARM64$' { $arch = 'arm64' }
    default { Die "unsupported architecture: $native (supported: amd64, arm64)" }
  }
}
if ($arch -ne 'amd64' -and $arch -ne 'arm64') { Die "unsupported architecture: $arch (supported: amd64, arm64)" }

$dir = $env:JITSO_INSTALL_DIR
if (-not $dir) {
  if (-not $env:LOCALAPPDATA) { Die 'LOCALAPPDATA is not set; set JITSO_INSTALL_DIR to the folder for jitso.exe' }
  $dir = Join-Path $env:LOCALAPPDATA 'Programs\jitso'
}
$target = Join-Path $dir 'jitso.exe'

function Test-OnPath([string]$Folder) {
  $wanted = $Folder.TrimEnd('\')
  foreach ($entry in ($env:Path -split ';')) {
    if ($entry.TrimEnd('\') -ieq $wanted) { return $true }
  }
  return $false
}

# Add-ToUserPath appends the install folder to the user PATH, once, when it is
# not on PATH already. JITSO_NO_MODIFY_PATH=1 only says how.
function Add-ToUserPath {
  if (Test-OnPath $dir) { return }
  $hint = "[Environment]::SetEnvironmentVariable('Path', `"$dir;`" + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')"
  if ($env:JITSO_NO_MODIFY_PATH -eq '1') {
    Say "$dir is not on your PATH; add it, for example: $hint"
    Say 'the hook that jitso setup hook registers runs the word jitso, so it needs jitso on PATH'
    return
  }
  $user = [Environment]::GetEnvironmentVariable('Path', 'User')
  $already = $false
  if ($user) {
    foreach ($entry in ($user -split ';')) {
      if ($entry.TrimEnd('\') -ieq $dir.TrimEnd('\')) { $already = $true }
    }
  }
  if (-not $already) {
    if ($user) { $user = $user.TrimEnd(';') + ';' + $dir } else { $user = $dir }
    try {
      [Environment]::SetEnvironmentVariable('Path', $user, 'User')
      Say "added $dir to your user PATH"
    } catch {
      Say "cannot change your user PATH ($($_.Exception.Message)); add it yourself: $hint"
      return
    }
  }
  $env:Path = $env:Path.TrimEnd(';') + ';' + $dir
  Say 'open a new terminal to use jitso everywhere; this one can use it now'
}

function Get-VersionLine([string]$Exe) {
  try { return ((& $Exe version 2>$null) | Select-Object -First 1) } catch { return $null }
}

if ($Version -ne 'latest' -and (Test-Path -LiteralPath $target)) {
  $current = Get-VersionLine $target
  if ($current -and $current.StartsWith("jitso $Version ")) {
    Say "jitso $Version is already installed at $target"
    Add-ToUserPath
    exit 0
  }
}

$base = $env:JITSO_DOWNLOAD_URL
if (-not $base) { $base = $releases }
$base = $base.TrimEnd('/', '\')
if ($Version -eq 'latest') { $from = "$base/latest/download" } else { $from = "$base/download/$Version" }

function Fetch([string]$Asset, [string]$Destination) {
  if ($base -match '^https://') {
    try { Invoke-WebRequest -UseBasicParsing -Uri "$from/$Asset" -OutFile $Destination }
    catch { Die "download failed: $from/${Asset}: $($_.Exception.Message)" }
  } elseif ($base -match '^file://') {
    $local = ([Uri]"$from/$Asset").LocalPath
    if (-not (Test-Path -LiteralPath $local)) { Die "cannot read $local" }
    Copy-Item -LiteralPath $local -Destination $Destination
  } elseif ([System.IO.Path]::IsPathRooted($base)) {
    if ($Version -eq 'latest') { $folder = Join-Path $base 'latest\download' } else { $folder = Join-Path $base "download\$Version" }
    $local = Join-Path $folder $Asset
    if (-not (Test-Path -LiteralPath $local)) { Die "cannot read $local" }
    Copy-Item -LiteralPath $local -Destination $Destination
  } else {
    Die "JITSO_DOWNLOAD_URL must be an https:// or file:// URL or an absolute folder: $base"
  }
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("jitso-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  $asset = "jitso-windows-$arch.zip"
  Fetch 'checksums.txt' (Join-Path $tmp 'checksums.txt')
  Fetch $asset (Join-Path $tmp $asset)

  $want = $null
  foreach ($line in (Get-Content -LiteralPath (Join-Path $tmp 'checksums.txt'))) {
    $parts = $line -split '\s+', 2
    if ($parts.Count -eq 2 -and ($parts[1] -eq $asset -or $parts[1] -eq "*$asset")) { $want = $parts[0]; break }
  }
  if (-not $want) { Die "checksums.txt has no line for $asset; nothing installed" }
  if ($want -notmatch '^[0-9a-f]{64}$') { Die "checksums.txt has a malformed line for $asset; nothing installed" }
  $got = (Get-FileHash -LiteralPath (Join-Path $tmp $asset) -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($got -ne $want) { Die "checksum mismatch for $asset (expected $want, got $got); nothing installed" }

  $x = Join-Path $tmp 'x'
  try { Expand-Archive -LiteralPath (Join-Path $tmp $asset) -DestinationPath $x }
  catch { Die "cannot extract $asset; nothing installed" }
  $new = Join-Path $x "jitso-windows-$arch\jitso.exe"
  if (-not (Test-Path -LiteralPath $new)) { Die "$asset has no jitso-windows-$arch\jitso.exe; nothing installed" }
  $line = Get-VersionLine $new
  if (-not $line) { Die 'the downloaded jitso does not run on this machine; nothing installed' }

  $old = $null
  if (Test-Path -LiteralPath $target) {
    $old = Get-VersionLine $target
    if ($old -eq $line) {
      Say "$line is already installed at $target"
      Add-ToUserPath
      exit 0
    }
  }

  try { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
  catch { Die "cannot create $dir; set JITSO_INSTALL_DIR to a folder you own" }
  $stage = Join-Path $dir ".jitso.tmp.$PID.exe"
  try { Copy-Item -LiteralPath $new -Destination $stage -Force }
  catch { Die "cannot write to $dir; set JITSO_INSTALL_DIR to a folder you own" }
  try { Move-Item -LiteralPath $stage -Destination $target -Force }
  catch {
    Remove-Item -LiteralPath $stage -Force -ErrorAction SilentlyContinue
    Die "cannot replace $target (is jitso running? close it and run again)"
  }

  if ($old) { Say "upgraded ${target}: $old -> $line" } else { Say "installed $line at $target" }
  Add-ToUserPath
  Say 'next: run jitso init in your project to set it up (skill, AGENTS.md block, guard)'
} finally {
  Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
