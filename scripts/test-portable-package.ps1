param([Parameter(Mandatory = $true)][string]$PackageDirectory)
$ErrorActionPreference = "Stop"

foreach ($directory in @("exes", "Logs")) {
    $path = Join-Path $PackageDirectory $directory
    if (-not (Test-Path -LiteralPath $path -PathType Container)) { throw "Portable package directory is missing: $path" }
}
foreach ($file in @("pca.cmd", "config.toml")) {
    $path = Join-Path $PackageDirectory $file
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Portable package file is missing: $path" }
}
foreach ($name in @("paneacea.exe", "paneacea-runtime.exe", "paneacea-cli.exe")) {
    $path = Join-Path (Join-Path $PackageDirectory "exes") $name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Portable package executable is missing: $path" }
    if ((Get-Item -LiteralPath $path).Length -le 0) { throw "Portable package executable is empty: $path" }
}
$launcher = Get-Content -LiteralPath (Join-Path $PackageDirectory "pca.cmd") -Raw
if ($launcher -notmatch 'exes\\paneacea\.exe') { throw "Portable launcher does not target exes\paneacea.exe." }
$config = Get-Content -LiteralPath (Join-Path $PackageDirectory "config.toml") -Raw
if ($config -notmatch '(?m)^\[settings\]\s*$') { throw "Portable config.toml does not contain a settings section." }
Write-Host "Portable package validation passed: $PackageDirectory"
