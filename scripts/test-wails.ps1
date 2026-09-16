. "$PSScriptRoot\wails-env.ps1"
. "$PSScriptRoot\test-stop-paneacea.ps1"
go test -timeout 90s ./internal/...
if ($LASTEXITCODE -ne 0) { throw "Go tests failed." }
Push-Location frontend
try {
    npm run check
    if ($LASTEXITCODE -ne 0) { throw "Frontend checks failed." }
    npm test
    if ($LASTEXITCODE -ne 0) { throw "Frontend tests failed." }
} finally { Pop-Location }
git diff --check
if ($LASTEXITCODE -ne 0) { throw "Whitespace checks failed." }
