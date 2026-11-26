# Tests scripts/install.ps1 without a network, against the Windows archives
# built by scripts/package.sh (the zip and checksums.txt of one release folder).
#
# Usage, from the repo root, in PowerShell 5.1 or 7:
#   .\scripts\test-install.ps1 -Dist <dist dir> -Tag v0.0.0-ci
#
# Every check prints "ok <name>" or "FAIL <name>"; the exit code is 1 when any
# check fails. It works in throwaway folders under the temp folder, and it
# restores the user PATH it changes. It runs install.ps1 in a child process of
# each PowerShell it finds (powershell.exe and pwsh).
param(
  [Parameter(Mandatory = $true)][string]$Dist,
  [Parameter(Mandatory = $true)][string]$Tag
)
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$installPs1 = Join-Path $root 'scripts\install.ps1'
if ($Tag -notmatch '^v\d+\.\d+\.\d+') { Write-Error "the tag must look like v1.2.3: $Tag"; exit 2 }
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$asset = "jitso-windows-$arch.zip"
if (-not ((Test-Path (Join-Path $Dist 'checksums.txt')) -and (Test-Path (Join-Path $Dist $asset)))) {
  Write-Error "$Dist has no checksums.txt or $asset"; exit 1
}

$shells = @()
foreach ($name in 'powershell', 'pwsh') {
  $cmd = Get-Command $name -ErrorAction SilentlyContinue
  if ($cmd) { $shells += $cmd.Source }
}
if ($shells.Count -eq 0) { Write-Error 'no PowerShell found'; exit 1 }

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("jitso-test-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp | Out-Null
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')

# Layout of a GitHub releases folder: download\<tag>\<assets> and latest\download\<assets>.
$rel = Join-Path $tmp 'releases'
New-Item -ItemType Directory -Path (Join-Path $rel "download\$Tag"), (Join-Path $rel 'latest\download') | Out-Null
Copy-Item -Path (Join-Path $Dist '*') -Destination (Join-Path $rel "download\$Tag")
Copy-Item -Path (Join-Path $Dist '*') -Destination (Join-Path $rel 'latest\download')

$script:failed = $false
function Pass([string]$Name) { Write-Host "ok   $Name" }
function Fail([string]$Name, [string]$Why) { Write-Host "FAIL ${Name}: $Why"; $script:failed = $true }

# Run-Install runs install.ps1 in a child of the given shell with the given
# variables, and returns the exit code and the text of both streams.
function Run-Install([string]$Shell, [hashtable]$Vars, [string[]]$Arguments = @()) {
  foreach ($k in 'JITSO_VERSION', 'JITSO_INSTALL_DIR', 'JITSO_DOWNLOAD_URL', 'JITSO_ARCH', 'JITSO_NO_MODIFY_PATH') {
    Remove-Item "Env:$k" -ErrorAction SilentlyContinue
  }
  foreach ($k in $Vars.Keys) { Set-Item "Env:$k" $Vars[$k] }
  $outFile = Join-Path $tmp 'out.txt'
  $errFile = Join-Path $tmp 'err.txt'
  $p = Start-Process -FilePath $Shell -Wait -PassThru -NoNewWindow `
    -ArgumentList (@('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$installPs1`"") + $Arguments) `
    -RedirectStandardOutput $outFile -RedirectStandardError $errFile
  foreach ($k in $Vars.Keys) { Remove-Item "Env:$k" -ErrorAction SilentlyContinue }
  return [pscustomobject]@{
    Code = $p.ExitCode
    Out  = (Get-Content -Raw -LiteralPath $outFile -ErrorAction SilentlyContinue)
    Err  = (Get-Content -Raw -LiteralPath $errFile -ErrorAction SilentlyContinue)
  }
}

try {
  # An older install for the upgrade check: a small program that prints another
  # version line, built with the C# compiler of the .NET Framework.
  $csc = Get-ChildItem -Path (Join-Path $env:WINDIR 'Microsoft.NET\Framework*\v4*\csc.exe') |
    Select-Object -First 1 -ExpandProperty FullName
  if (-not $csc) { Write-Error 'no csc.exe to build the older jitso.exe' }
  $oldSource = Join-Path $tmp 'old.cs'
  $oldExe = Join-Path $tmp 'old.exe'
  Set-Content -LiteralPath $oldSource -Value 'class P { static void Main() { System.Console.WriteLine("jitso v0.0.0-old (fake)"); } }'
  & $csc /nologo "/out:$oldExe" $oldSource
  if ($LASTEXITCODE -ne 0) { Write-Error 'cannot build the older jitso.exe' }

  $n = 0
  foreach ($shell in $shells) {
    $n++
    $label = [IO.Path]::GetFileNameWithoutExtension($shell)
    $base = @{ JITSO_DOWNLOAD_URL = $rel; JITSO_NO_MODIFY_PATH = '1' }

    # 1 pinned install into a folder with a space in its name
    $dir = Join-Path $tmp "bin $n"
    $r = Run-Install $shell ($base + @{ JITSO_INSTALL_DIR = $dir }) @($Tag)
    $exe = Join-Path $dir 'jitso.exe'
    if ($r.Code -ne 0) { Fail "$label pinned install" "exit $($r.Code): $($r.Out) $($r.Err)" }
    elseif (-not (Test-Path $exe)) { Fail "$label pinned install" 'no jitso.exe' }
    else {
      $v = (& $exe version | Select-Object -First 1)
      if ($v -like "jitso $Tag *") { Pass "$label pinned install" } else { Fail "$label pinned install" "version line: $v" }
    }
    if ($r.Out -like "*is not on your PATH*") { Pass "$label no PATH edit says how" } else { Fail "$label no PATH edit says how" $r.Out }
    # only an upgrade says how to refresh the installed user skill
    $refresh = '*jitso install skill --scope user*'
    if ($r.Out -notlike $refresh) { Pass "$label fresh install says nothing about the user skill" } else { Fail "$label fresh install says nothing about the user skill" $r.Out }

    # 2 the same version again changes nothing
    $r = Run-Install $shell ($base + @{ JITSO_INSTALL_DIR = $dir }) @($Tag)
    if ($r.Code -eq 0 -and $r.Out -like '*is already installed*') { Pass "$label second run" } else { Fail "$label second run" "exit $($r.Code): $($r.Out)" }

    # 3 latest, from the version in the environment
    $dir2 = Join-Path $tmp "latest $n"
    $r = Run-Install $shell ($base + @{ JITSO_INSTALL_DIR = $dir2 })
    if ($r.Code -eq 0 -and (Test-Path (Join-Path $dir2 'jitso.exe'))) { Pass "$label latest" } else { Fail "$label latest" "exit $($r.Code): $($r.Out) $($r.Err)" }

    # 4 a bad checksum installs nothing
    $bad = Join-Path $tmp "bad$n"
    New-Item -ItemType Directory -Path (Join-Path $bad "download\$Tag") | Out-Null
    Copy-Item -Path (Join-Path $Dist '*') -Destination (Join-Path $bad "download\$Tag")
    $sums = Join-Path $bad "download\$Tag\checksums.txt"
    (Get-Content $sums) -replace '^[0-9a-f]{64}', ('0' * 64) | Set-Content $sums
    $dir3 = Join-Path $tmp "badbin $n"
    $r = Run-Install $shell @{ JITSO_DOWNLOAD_URL = $bad; JITSO_NO_MODIFY_PATH = '1'; JITSO_INSTALL_DIR = $dir3 } @($Tag)
    if ($r.Code -ne 0 -and $r.Err -like '*checksum mismatch*' -and -not (Test-Path (Join-Path $dir3 'jitso.exe'))) { Pass "$label checksum mismatch" }
    else { Fail "$label checksum mismatch" "exit $($r.Code), exists $(Test-Path (Join-Path $dir3 'jitso.exe')): $($r.Err)" }

    # 5 a version that is not a version exits 2
    $r = Run-Install $shell ($base + @{ JITSO_INSTALL_DIR = (Join-Path $tmp "none $n") }) @('banana')
    if ($r.Code -eq 2 -and $r.Err -like '*not a version*') { Pass "$label bad version" } else { Fail "$label bad version" "exit $($r.Code): $($r.Err)" }

    # 6 an unsupported architecture exits 1 and installs nothing
    $r = Run-Install $shell ($base + @{ JITSO_INSTALL_DIR = (Join-Path $tmp "none $n"); JITSO_ARCH = 'riscv' }) @($Tag)
    if ($r.Code -eq 1 -and $r.Err -like '*unsupported architecture*') { Pass "$label bad arch" } else { Fail "$label bad arch" "exit $($r.Code): $($r.Err)" }

    # 7 the user PATH gets the folder once, and the run is idempotent
    $dir4 = Join-Path $tmp "pathbin $n"
    $pathVars = @{ JITSO_DOWNLOAD_URL = $rel; JITSO_INSTALL_DIR = $dir4 }
    [Environment]::SetEnvironmentVariable('Path', $userPath, 'User')
    $r = Run-Install $shell $pathVars @($Tag)
    $r2 = Run-Install $shell $pathVars @($Tag)
    $entries = ([Environment]::GetEnvironmentVariable('Path', 'User') -split ';') | Where-Object { $_.TrimEnd('\') -ieq $dir4 }
    if ($r.Code -eq 0 -and $r2.Code -eq 0 -and @($entries).Count -eq 1) { Pass "$label user PATH once" }
    else { Fail "$label user PATH once" "exit $($r.Code)/$($r2.Code), entries $(@($entries).Count): $($r.Out) $($r.Err)" }
    [Environment]::SetEnvironmentVariable('Path', $userPath, 'User')

    # 8 an upgrade says how to refresh the installed user skill
    $dir5 = Join-Path $tmp "upgrade $n"
    New-Item -ItemType Directory -Path $dir5 | Out-Null
    Copy-Item -LiteralPath $oldExe -Destination (Join-Path $dir5 'jitso.exe')
    $r = Run-Install $shell ($base + @{ JITSO_INSTALL_DIR = $dir5 }) @($Tag)
    if ($r.Code -eq 0 -and $r.Out -like '*upgraded*' -and $r.Out -like $refresh) { Pass "$label upgrade says how to refresh the user skill" }
    else { Fail "$label upgrade says how to refresh the user skill" "exit $($r.Code): $($r.Out) $($r.Err)" }
  }
} finally {
  [Environment]::SetEnvironmentVariable('Path', $userPath, 'User')
  Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
if ($script:failed) { exit 1 }
