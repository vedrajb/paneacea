$ErrorActionPreference = "Stop"

$psContents = Get-Content -LiteralPath (Join-Path $PSScriptRoot "stop-paneacea.ps1") -Raw
if ($psContents -notmatch 'paneacea\.exe' -or $psContents -notmatch 'MainModule\.FileName' -or $psContents -notmatch 'Stop-Process') {
    throw "stop-paneacea.ps1 does not stop paneacea.exe by path."
}
if ($psContents -match 'paneacea-runtime|paneacea-cli') {
    throw "stop-paneacea.ps1 must not target other Paneacea processes."
}
$batContents = Get-Content -LiteralPath (Join-Path $PSScriptRoot "stop-paneacea.bat") -Raw
if ($batContents -notmatch 'stop-paneacea\.ps1') {
    throw "stop-paneacea.bat does not delegate to stop-paneacea.ps1."
}
$wailsContents = Get-Content -LiteralPath (Join-Path $PSScriptRoot "stop-wails.ps1") -Raw
if ($wailsContents -notmatch [regex]::Escape('stop-paneacea.ps1')) {
    throw "stop-wails.ps1 does not reuse stop-paneacea.ps1."
}
$package = Get-Content -LiteralPath (Join-Path $PSScriptRoot "..\package.json") -Raw | ConvertFrom-Json
if ($package.scripts.stop -ne "scripts\stop-paneacea.bat") {
    throw "package.json does not expose the Paneacea stop command."
}

Write-Host "Paneacea stop script validation passed."
