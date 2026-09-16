function Stop-WailsProcesses {
    param([Parameter(Mandatory = $true)][string]$BuildDirectory)

    & "$PSScriptRoot\stop-paneacea.bat"
    if ($LASTEXITCODE -ne 0) { throw "Unable to stop Paneacea processes before building." }
}
