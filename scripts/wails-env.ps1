$ErrorActionPreference = "Stop"
$projectDirectory = Split-Path $PSScriptRoot -Parent
Set-Location $projectDirectory
$localGo = Join-Path $projectDirectory ".build-validation\go-toolchain\go\bin\go.exe"
if (Test-Path -LiteralPath $localGo) {
    $env:PATH = (Split-Path $localGo -Parent) + ";" + $env:PATH
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Install Go 1.24.2 or newer and reopen PowerShell."
}
foreach ($directory in @(".build-validation\gopath", ".build-validation\gocache", ".build-validation\tmp", "build\bin")) {
    if (-not (Test-Path -LiteralPath $directory)) { New-Item -ItemType Directory -Path $directory | Out-Null }
}
$env:GOPATH = Join-Path $projectDirectory ".build-validation\gopath"
$env:GOCACHE = Join-Path $projectDirectory ".build-validation\gocache"
$env:GOTMPDIR = Join-Path $projectDirectory ".build-validation\tmp"
$env:TEMP = $env:GOTMPDIR
$env:TMP = $env:GOTMPDIR
$env:npm_config_cache = Join-Path $projectDirectory ".build-validation\npm-cache"
