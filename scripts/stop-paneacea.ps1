param([string]$PortableDirectory = (Join-Path $PSScriptRoot "..\paneacea-portable"))

# Stop only the paneacea.exe that runs from the paneacea-portable folder.
$exePath = [System.IO.Path]::GetFullPath((Join-Path $PortableDirectory "paneacea.exe"))
$stopped = 0
$failed = $false
foreach ($process in @(Get-Process -Name "paneacea" -ErrorAction SilentlyContinue)) {
    $path = $null
    try { $path = $process.MainModule.FileName } catch { continue }
    if ($path -and [string]::Equals([System.IO.Path]::GetFullPath($path), $exePath, [System.StringComparison]::OrdinalIgnoreCase)) {
        Write-Host "Closing $exePath (PID $($process.Id))..."
        try { Stop-Process -Id $process.Id -Force -ErrorAction Stop; $stopped++ }
        catch { Write-Host "Failed to stop PID $($process.Id)."; $failed = $true }
    }
}
if ($stopped -eq 0 -and -not $failed) { Write-Host "No paneacea.exe is running from paneacea-portable." }
if ($failed) { exit 1 }
exit 0
