param([switch]$Run)
. "$PSScriptRoot\wails-env.ps1"
. "$PSScriptRoot\stop-wails.ps1"
Stop-WailsProcesses -BuildDirectory (Join-Path $projectDirectory "build\bin")
Push-Location frontend
try {
    if (-not (Test-Path node_modules)) { npm ci; if ($LASTEXITCODE -ne 0) { throw "Frontend dependency installation failed." } }
    npm run check
    if ($LASTEXITCODE -ne 0) { throw "Frontend checks failed." }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "Frontend build failed." }
} finally { Pop-Location }
go build -trimpath -o build/bin/paneacea-runtime.exe ./cmd/paneacea-runtime
if ($LASTEXITCODE -ne 0) { throw "Runtime build failed." }
go build -trimpath -o build/bin/paneacea-cli.exe ./cmd/paneacea-cli
if ($LASTEXITCODE -ne 0) { throw "CLI build failed." }
go build -trimpath -tags desktop,production -ldflags "-H windowsgui" -o build/bin/paneacea.exe ./cmd/paneacea
if ($LASTEXITCODE -ne 0) { throw "Desktop build failed." }
Write-Host "Built build\bin\paneacea.exe, paneacea-runtime.exe, and paneacea-cli.exe"
if ($Run) { Start-Process -FilePath .\build\bin\paneacea.exe -WindowStyle Normal }
