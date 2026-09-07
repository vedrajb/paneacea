$ErrorActionPreference = 'Stop'
if (!(Test-Path Cargo.toml)) { throw 'Run this script from the tinkershell repository directory.' }
$env:CARGO_HOME = Join-Path $PWD '.cargo-home'
$env:NUGET_PACKAGES = Join-Path $PWD '.packages'
cargo test --workspace
if ($LASTEXITCODE -ne 0) { throw 'Rust tests failed.' }
dotnet run --project tests/Terminal.Core.Tests/Terminal.Core.Tests.csproj
if ($LASTEXITCODE -ne 0) { throw 'Core tests failed.' }
dotnet run --project tests/TerminalApp.Smoke/TerminalApp.Smoke.csproj
if ($LASTEXITCODE -ne 0) { throw 'WPF smoke test failed.' }
