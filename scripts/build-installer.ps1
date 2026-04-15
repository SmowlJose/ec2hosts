<#
.SYNOPSIS
  Build the Windows GUI installer locally (CLI staged, NSIS invoked).

.DESCRIPTION
  Mirrors the steps in .github/workflows/release.yml's gui-build job so
  `wails build -nsis` actually produces an installer locally. Without
  this, NSIS aborts on project.nsi line 73 with "no files found" for
  ..\..\bin\ec2hosts.exe — the CLI binary that the installer bundles
  alongside the GUI for UAC elevation.

  Pipeline:
    1. Cross-check prerequisites are in PATH (go, wails, makensis).
    2. Build the CLI -> cmd\gui\build\bin\ec2hosts.exe   (staged).
    3. Copy config.yaml.example next to project.nsi       (staged).
    4. Run `wails build -platform windows/amd64 -nsis`.
    5. Report the three artifacts with their sizes.

.PARAMETER Clean
  Wipe cmd\gui\build\bin\* before building. Use when a previous build
  left stale artifacts you want to be certain aren't in the output.

.PARAMETER SkipInstaller
  Run only steps 1-3 and the wails build, but pass no -nsis flag. Useful
  when iterating on the GUI and you don't need the 14 MB installer on
  every cycle.

.EXAMPLE
  # From anywhere inside the repo:
  .\scripts\build-installer.ps1

.EXAMPLE
  # Cold build from a clean bin directory:
  .\scripts\build-installer.ps1 -Clean

.EXAMPLE
  # GUI only, no installer:
  .\scripts\build-installer.ps1 -SkipInstaller

.NOTES
  Requires Go, Wails v2 CLI, and NSIS (makensis.exe) in PATH. See
  README.md for one-liners that add the usual install locations to
  your user PATH.
#>

[CmdletBinding()]
param(
  [switch]$Clean,
  [switch]$SkipInstaller
)

# Fail fast on any cmdlet error. External commands (go/wails/makensis)
# still need explicit $LASTEXITCODE checks because PowerShell does not
# propagate native exit codes to $ErrorActionPreference.
$ErrorActionPreference = 'Stop'

# Resolve the repo root so the script works from any cwd inside a
# worktree, and from outside via its absolute path. Prefer `git`, fall
# back to walking up from the script's own location.
function Get-RepoRoot {
  try {
    $root = git rev-parse --show-toplevel 2>$null
    if ($LASTEXITCODE -eq 0 -and $root) {
      return (Resolve-Path $root).Path
    }
  } catch {
    # git missing or not a repo — fall through to the path-based resolver
  }
  return (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
}

function Assert-Tool($name, $hint) {
  if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
    throw "$name not found in PATH. $hint"
  }
}

function Write-Step($msg) {
  Write-Host ("--> " + $msg) -ForegroundColor Cyan
}

$Repo = Get-RepoRoot
Push-Location $Repo
try {
  Write-Host ("repo: " + $Repo) -ForegroundColor DarkGray

  # ------------------------------------------------------------------
  # 1. Prerequisites
  # ------------------------------------------------------------------
  Assert-Tool 'go'       'Install Go from https://go.dev/dl and ensure "C:\Program Files\Go\bin" is on your PATH.'
  Assert-Tool 'wails'    'Install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest  (adds to %USERPROFILE%\go\bin)'
  if (-not $SkipInstaller) {
    Assert-Tool 'makensis' 'Install NSIS from https://nsis.sourceforge.io and add "C:\Program Files (x86)\NSIS" to your PATH.'
  }

  # ------------------------------------------------------------------
  # 2. Optional clean of the bin directory
  # ------------------------------------------------------------------
  $binDir = Join-Path $Repo 'cmd\gui\build\bin'
  if ($Clean -and (Test-Path $binDir)) {
    Write-Step "cleaning $binDir"
    Remove-Item -Recurse -Force -Path (Join-Path $binDir '*')
  }

  # ------------------------------------------------------------------
  # 3. Stage the CLI binary where project.nsi expects it
  # ------------------------------------------------------------------
  # Use the same flags the release workflow uses so local + CI builds
  # produce byte-identical binaries (modulo timestamps): -trimpath
  # strips local paths and -s -w drops the debug/symbol tables.
  Write-Step 'building CLI -> cmd\gui\build\bin\ec2hosts.exe'
  $env:GOOS        = 'windows'
  $env:GOARCH      = 'amd64'
  $env:CGO_ENABLED = '0'
  go build -trimpath -ldflags '-s -w' -o 'cmd\gui\build\bin\ec2hosts.exe' './cmd/cli'
  if ($LASTEXITCODE -ne 0) { throw "go build failed (exit $LASTEXITCODE)" }

  # ------------------------------------------------------------------
  # 4. Stage config.yaml.example next to project.nsi
  # ------------------------------------------------------------------
  # project.nsi references this via a relative File directive, so the
  # .yaml has to live next to the .nsi — not at the repo root.
  $exampleSrc = Join-Path $Repo 'config.yaml.example'
  $exampleDst = Join-Path $Repo 'cmd\gui\build\windows\installer\config.yaml.example'
  Write-Step 'staging config.yaml.example -> cmd\gui\build\windows\installer\'
  Copy-Item -Path $exampleSrc -Destination $exampleDst -Force

  # ------------------------------------------------------------------
  # 5. Wails build (+ NSIS unless -SkipInstaller)
  # ------------------------------------------------------------------
  # Wails' own build flags (-clean, -race, etc.) stay off: -clean would
  # wipe our staged CLI, and the CI workflow avoids it for the same
  # reason. Fresh checkouts don't need cleaning.
  Push-Location (Join-Path $Repo 'cmd\gui')
  try {
    if ($SkipInstaller) {
      Write-Step 'wails build -platform windows/amd64'
      wails build -platform windows/amd64
    } else {
      Write-Step 'wails build -platform windows/amd64 -nsis'
      wails build -platform windows/amd64 -nsis
    }
    if ($LASTEXITCODE -ne 0) { throw "wails build failed (exit $LASTEXITCODE)" }
  } finally {
    Pop-Location
  }

  # ------------------------------------------------------------------
  # 6. Report artifacts
  # ------------------------------------------------------------------
  $artifacts = @('ec2hosts-gui.exe', 'ec2hosts.exe')
  if (-not $SkipInstaller) { $artifacts += 'ec2hosts-amd64-installer.exe' }

  Write-Host ''
  Write-Host '---------------------------------------------' -ForegroundColor DarkGray
  Write-Host 'Artifacts' -ForegroundColor Green
  foreach ($name in $artifacts) {
    $path = Join-Path $binDir $name
    if (Test-Path $path) {
      $f = Get-Item $path
      $sizeMB = [math]::Round($f.Length / 1MB, 1)
      Write-Host ("  {0,-34} {1,6} MB" -f $f.Name, $sizeMB)
    } else {
      Write-Host ("  {0,-34} MISSING" -f $name) -ForegroundColor Red
    }
  }
  Write-Host '---------------------------------------------' -ForegroundColor DarkGray
  if (-not $SkipInstaller) {
    Write-Host ("Installer: " + (Join-Path $binDir 'ec2hosts-amd64-installer.exe')) -ForegroundColor Green
  }
}
finally {
  Pop-Location
}
