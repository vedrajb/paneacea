$ErrorActionPreference = "Stop"

$scriptPath = Join-Path $PSScriptRoot "stop-paneacea.bat"
$contents = Get-Content -LiteralPath $scriptPath -Raw
foreach ($name in @("paneacea.exe", "paneacea-runtime.exe", "paneacea-cli.exe")) {
    if ($contents -notmatch [regex]::Escape($name)) {
        throw "stop-paneacea.bat does not target $name."
    }
}
if ($contents -notmatch 'taskkill /F /IM "%%P"') {
    throw "stop-paneacea.bat does not stop matching processes."
}
$wailsContents = Get-Content -LiteralPath (Join-Path $PSScriptRoot "stop-wails.ps1") -Raw
if ($wailsContents -notmatch [regex]::Escape('& "$PSScriptRoot\stop-paneacea.bat"')) {
    throw "stop-wails.ps1 does not reuse stop-paneacea.bat."
}
$package = Get-Content -LiteralPath (Join-Path $PSScriptRoot "..\package.json") -Raw | ConvertFrom-Json
if ($package.scripts.stop -ne "scripts\stop-paneacea.bat") {
    throw "package.json does not expose the Paneacea stop command."
}

Write-Host "Paneacea stop script validation passed."
