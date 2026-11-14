# Build devdeck for Windows (default) or every release target.
#   ./scripts/build.ps1            -> ./devdeck.exe
#   ./scripts/build.ps1 -All       -> ./dist/<os>_<arch>/devdeck[.exe]
param([switch]$All, [string]$Version = "")

$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot -Parent)
if (-not $Version) { $Version = (git describe --tags --always --dirty 2>$null); if (-not $Version) { $Version = "dev" } }
$env:CGO_ENABLED = "0"
$ld = "-s -w -X main.version=$Version"

if (-not $All) {
  go build -trimpath -ldflags $ld -o devdeck.exe .
  Write-Host "built devdeck.exe ($Version)"
  return
}
$targets = "windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"
foreach ($t in $targets) {
  $os, $arch = $t.Split("/")
  $env:GOOS = $os; $env:GOARCH = $arch
  $ext = if ($os -eq "windows") { ".exe" } else { "" }
  go build -trimpath -ldflags $ld -o "dist/${os}_${arch}/devdeck$ext" .
  Write-Host "built dist/${os}_${arch}"
}
Remove-Item Env:GOOS, Env:GOARCH
