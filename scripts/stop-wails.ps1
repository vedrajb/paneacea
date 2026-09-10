function Stop-WailsProcesses {
    param([Parameter(Mandatory = $true)][string]$BuildDirectory)

    foreach ($name in @("paneacea", "paneacea-cli", "paneacea-runtime")) {
        $executable = Join-Path $BuildDirectory "$name.exe"
        foreach ($process in @(Get-Process -Name $name -ErrorAction SilentlyContinue)) {
            if ($process.Path -ne $executable) { continue }
            Write-Host "Closing $name (PID $($process.Id)) before building."
            try {
                Stop-Process -InputObject $process -ErrorAction Stop
                if (-not $process.WaitForExit(10000)) {
                    throw "Timed out waiting for $name (PID $($process.Id)) to exit."
                }
            } catch {
                if (-not $process.HasExited) { throw }
            }
        }
    }
}
