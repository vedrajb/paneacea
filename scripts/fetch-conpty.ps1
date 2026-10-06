param(
    [Parameter(Mandatory = $true)][string[]]$Destination
)
$ErrorActionPreference = "Stop"

# Pinned Microsoft.Windows.Console.ConPTY release. The bundled ConPTY passes application VT modes
# (mouse tracking, etc.) through to the terminal, which the inbox Windows ConPTY drops.
$version = "1.25.260930003"
$packageSha256 = "02B07B349AF66D801159BDF9E440D4A1CE78BB951F37FC8609731665AFDAE7EE"
$projectDirectory = Split-Path $PSScriptRoot -Parent
$cacheDirectory = Join-Path $projectDirectory ".packages\conpty\$version"
$packagePath = Join-Path $cacheDirectory "microsoft.windows.console.conpty.$version.zip"
$expandedDirectory = Join-Path $cacheDirectory "package"
$packageUrl = "https://api.nuget.org/v3-flatcontainer/microsoft.windows.console.conpty/$version/microsoft.windows.console.conpty.$version.nupkg"

function Test-PackageHash {
    (Test-Path -LiteralPath $packagePath -PathType Leaf) -and
        ((Get-FileHash -LiteralPath $packagePath -Algorithm SHA256).Hash -eq $packageSha256)
}

if (-not (Test-Path -LiteralPath $cacheDirectory)) { New-Item -ItemType Directory -Path $cacheDirectory | Out-Null }
if (-not (Test-PackageHash)) {
    Write-Host "Downloading ConPTY $version."
    $download = "$packagePath.download"
    Invoke-WebRequest -UseBasicParsing -Uri $packageUrl -OutFile $download
    Move-Item -LiteralPath $download -Destination $packagePath -Force
    if (-not (Test-PackageHash)) { throw "ConPTY package hash mismatch: $packagePath" }
}

$files = @{
    "conpty.dll"      = Join-Path $expandedDirectory "runtimes\win-x64\native\conpty.dll"
    "OpenConsole.exe" = Join-Path $expandedDirectory "build\native\runtimes\x64\OpenConsole.exe"
}
if (@($files.Values | Where-Object { -not (Test-Path -LiteralPath $_ -PathType Leaf) }).Count -gt 0) {
    Expand-Archive -LiteralPath $packagePath -DestinationPath $expandedDirectory -Force
}
foreach ($source in $files.Values) {
    $signature = Get-AuthenticodeSignature -LiteralPath $source
    if ($signature.Status -ne "Valid" -or $signature.SignerCertificate.Subject -notmatch '^CN=Microsoft Corporation,') {
        throw "ConPTY binary is not signed by Microsoft: $source"
    }
}
$files["conpty-LICENSE.txt"] = Join-Path $PSScriptRoot "conpty-LICENSE.txt"

foreach ($directory in $Destination) {
    if (-not (Test-Path -LiteralPath $directory)) { New-Item -ItemType Directory -Path $directory | Out-Null }
    foreach ($name in $files.Keys) {
        $target = Join-Path $directory $name
        # Skip identical files so a rebuild does not fail on an OpenConsole.exe that is still in use.
        if ((Test-Path -LiteralPath $target -PathType Leaf) -and
            (Get-FileHash -LiteralPath $target).Hash -eq (Get-FileHash -LiteralPath $files[$name]).Hash) { continue }
        Copy-Item -LiteralPath $files[$name] -Destination $target -Force
    }
}
