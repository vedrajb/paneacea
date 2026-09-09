param([ValidateSet('Debug', 'Release')][string]$Configuration = 'Debug')
$ErrorActionPreference = 'Stop'
if (!(Test-Path Cargo.toml)) { throw 'Run this script from the Paneacea repository directory.' }
$env:CARGO_HOME = Join-Path $PWD '.cargo-home'
$env:NUGET_PACKAGES = Join-Path $PWD '.packages'
$cargoArguments = @('build', '--workspace')
if ($Configuration -eq 'Release') { $cargoArguments += '--release' }
& cargo @cargoArguments
if ($LASTEXITCODE -ne 0) { throw 'Rust build failed.' }
dotnet build src/TerminalApp/TerminalApp.csproj -c $Configuration -p:NuGetAudit=false
if ($LASTEXITCODE -ne 0) { throw 'WPF build failed.' }
$profile = $Configuration.ToLowerInvariant()
$buildOutput = "src/TerminalApp/bin/$Configuration/net9.0-windows/win-x64"
$output = Join-Path $PWD 'portable-release'
if (!(Test-Path $output)) { New-Item -ItemType Directory -Path $output | Out-Null }
Copy-Item "$buildOutput\*" $output -Exclude @('*.log', '*.db', '*.db-*')
Copy-Item "target/$profile/panacea-runtime.exe" $output
Copy-Item "target/$profile/paneacea.exe" $output
Copy-Item "scripts/pcaa.cmd" $output
$logs = Join-Path $output 'logs'
if (!(Test-Path $logs)) { New-Item -ItemType Directory -Path $logs | Out-Null }
Write-Host "Built $output/Paneacea.App.exe"
