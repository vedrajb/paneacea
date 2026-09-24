param(
    [Parameter(Mandatory = $true)][string]$PackageDirectory,
    [switch]$AllowPersistentState
)
$ErrorActionPreference = "Stop"

$packageScript = Get-Content -LiteralPath (Join-Path $PSScriptRoot "..\package.bat") -Raw
foreach ($expected in @('set "PACKAGE_DIRECTORY=%~dp0paneacea-portable"', 'set "ARCHIVE_PATH=%~dp0paneacea-portable.zip"')) {
    if (-not $packageScript.Contains($expected)) { throw "package.bat does not target $expected." }
}
$buildScript = Get-Content -LiteralPath (Join-Path $PSScriptRoot "build-wails.ps1") -Raw
if (-not $buildScript.Contains('paneacea-portable')) { throw "build-wails.ps1 does not target paneacea-portable." }
$removalIndex = $packageScript.IndexOf("Remove-Item")
if ($removalIndex -lt 0) { throw "package.bat does not contain the portable package removal command." }
$packageProtection = $packageScript.Substring(0, $removalIndex)
foreach ($name in @("paneacea.db", "paneacea.db-wal", "paneacea.db-shm")) {
    if ($packageProtection -notmatch [regex]::Escape($name)) { throw "package.bat does not check $name before removal." }
}
if ($packageProtection -notmatch 'Test-Path -LiteralPath \$path' -or $packageProtection -notmatch 'Refusing to remove portable package' -or $packageProtection -notmatch 'exit /b %EXIT_CODE%') {
    throw "package.bat does not have a refusal path for persistent portable state."
}

if (-not $AllowPersistentState) {
    foreach ($name in @("paneacea.db", "paneacea.db-wal", "paneacea.db-shm")) {
        $path = Join-Path $PackageDirectory $name
        if (Test-Path -LiteralPath $path) { throw "Portable package contains persistent state: $path" }
    }
}

foreach ($directory in @("Logs")) {
    $path = Join-Path $PackageDirectory $directory
    if (-not (Test-Path -LiteralPath $path -PathType Container)) { throw "Portable package directory is missing: $path" }
}
$legacyExesDirectory = Join-Path $PackageDirectory "exes"
if (Test-Path -LiteralPath $legacyExesDirectory -PathType Container) { throw "Portable package contains the legacy exes directory: $legacyExesDirectory" }
foreach ($file in @("pca.cmd", "config.toml")) {
    $path = Join-Path $PackageDirectory $file
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Portable package file is missing: $path" }
}
foreach ($name in @("paneacea.exe", "paneacea-runtime.exe", "paneacea-cli.exe")) {
    $path = Join-Path $PackageDirectory $name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Portable package executable is missing: $path" }
    if ((Get-Item -LiteralPath $path).Length -le 0) { throw "Portable package executable is empty: $path" }
}
$launcher = Get-Content -LiteralPath (Join-Path $PackageDirectory "pca.cmd") -Raw
if ($launcher -notmatch '%~dp0paneacea\.exe') { throw "Portable launcher does not target paneacea.exe in the package root." }
$config = Get-Content -LiteralPath (Join-Path $PackageDirectory "config.toml") -Raw
if ($config -notmatch '(?m)^\[settings\]\s*$') { throw "Portable config.toml does not contain a settings section." }
Write-Host "Portable package validation passed: $PackageDirectory"
