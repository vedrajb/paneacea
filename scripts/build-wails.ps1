param(
    [ValidateSet("Debug", "Release")]
    [string]$Configuration = "Release",
    [switch]$Run
)
. "$PSScriptRoot\wails-env.ps1"
. "$PSScriptRoot\stop-wails.ps1"
Stop-WailsProcesses -BuildDirectory (Join-Path $projectDirectory "build\bin")
Stop-WailsProcesses -BuildDirectory (Join-Path $projectDirectory "portable-release")
Write-Host "Building $Configuration configuration."
Push-Location frontend
try {
    if (-not (Test-Path node_modules)) { npm ci; if ($LASTEXITCODE -ne 0) { throw "Frontend dependency installation failed." } }
    npm run check
    if ($LASTEXITCODE -ne 0) { throw "Frontend checks failed." }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "Frontend build failed." }
} finally { Pop-Location }
node (Join-Path $projectDirectory "frontend\scripts\generate-windows-icon.mjs")
if ($LASTEXITCODE -ne 0) { throw "Windows icon generation failed." }
go run .\scripts\generate-windows-resource.go -icon .\build\icon-assets\paneacea.ico -out .\cmd\paneacea\paneacea_windows_amd64.syso
if ($LASTEXITCODE -ne 0) { throw "Windows resource generation failed." }
go build -trimpath -o build/bin/paneacea-runtime.exe ./cmd/paneacea-runtime
if ($LASTEXITCODE -ne 0) { throw "Runtime build failed." }
go build -trimpath -o build/bin/paneacea-cli.exe ./cmd/paneacea-cli
if ($LASTEXITCODE -ne 0) { throw "CLI build failed." }
if ($Configuration -eq "Release") {
    go build -trimpath -tags desktop,production -ldflags "-H windowsgui" -o build/bin/paneacea.exe ./cmd/paneacea
} else {
    go build -trimpath -tags desktop -o build/bin/paneacea.exe ./cmd/paneacea
}
if ($LASTEXITCODE -ne 0) { throw "Desktop build failed." }
$portableDirectory = Join-Path $projectDirectory "portable-release"
$portableLogsDirectory = Join-Path $portableDirectory "Logs"
foreach ($directory in @($portableDirectory, $portableLogsDirectory)) {
    if (-not (Test-Path -LiteralPath $directory)) { New-Item -ItemType Directory -Path $directory | Out-Null }
}
foreach ($name in @("paneacea.exe", "paneacea-runtime.exe", "paneacea-cli.exe")) {
    Copy-Item -LiteralPath (Join-Path $projectDirectory "build\bin\$name") -Destination $portableDirectory -Force
}
Copy-Item -LiteralPath (Join-Path $PSScriptRoot "pca.cmd") -Destination (Join-Path $portableDirectory "pca.cmd") -Force
$portableConfig = Join-Path $portableDirectory "config.toml"
if (-not (Test-Path -LiteralPath $portableConfig)) {
    @'
[settings]
default_shell = "auto"
scrollback = 10000
font_size = 13
theme = "dark"
keybindings = {}
'@ | Set-Content -LiteralPath $portableConfig -Encoding UTF8
}
& (Join-Path $PSScriptRoot "test-portable-package.ps1") -PackageDirectory $portableDirectory
Write-Host "Built $portableDirectory\pca.cmd"
if ($Run) { Start-Process -FilePath (Join-Path $portableDirectory "paneacea.exe") -WorkingDirectory $portableDirectory -WindowStyle Normal }
