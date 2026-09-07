param([ValidateSet('Debug', 'Release')][string]$Configuration = 'Debug')
$ErrorActionPreference = 'Stop'
if (!(Test-Path Cargo.toml)) { throw 'Run this script from the tinkershell repository directory.' }
$env:CARGO_HOME = Join-Path $PWD '.cargo-home'
$env:NUGET_PACKAGES = Join-Path $PWD '.packages'
$cargoArguments = @('build', '--workspace')
if ($Configuration -eq 'Release') { $cargoArguments += '--release' }
& cargo @cargoArguments
if ($LASTEXITCODE -ne 0) { throw 'Rust build failed.' }
dotnet build src/TerminalApp/TerminalApp.csproj -c $Configuration
if ($LASTEXITCODE -ne 0) { throw 'WPF build failed.' }
$profile = $Configuration.ToLowerInvariant()
$output = "src/TerminalApp/bin/$Configuration/net9.0-windows/win-x64"
Copy-Item "target/$profile/mux-runtime.exe" $output
Copy-Item "target/$profile/tinkershell.exe" $output
Write-Host "Built $output/Paneacea.App.exe"
