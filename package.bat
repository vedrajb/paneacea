@echo off
setlocal

if /I "%~1"=="--help" (
    echo Usage: package.bat
    echo Deletes the existing portable package, rebuilds it, and creates paneacea-portable-YYYY-MM-DD.zip.
    exit /b 0
)

if not "%~1"=="" (
    echo Unknown option: "%~1"
    echo Usage: package.bat
    exit /b 1
)

set "PACKAGE_DIRECTORY=%~dp0paneacea-portable"
rem Locale-independent date for the archive name, e.g. paneacea-portable-2026-10-07.zip.
for /f %%D in ('powershell.exe -NoProfile -Command "Get-Date -Format yyyy-MM-dd"') do set "PACKAGE_DATE=%%D"
if not defined PACKAGE_DATE (
    echo Could not determine the current date.
    exit /b 1
)
set "ARCHIVE_PATH=%~dp0paneacea-portable-%PACKAGE_DATE%.zip"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$protectedFiles = @('paneacea.db', 'paneacea.db-wal', 'paneacea.db-shm', 'bash-history'); $found = @(); foreach ($name in $protectedFiles) { $path = Join-Path -Path $env:PACKAGE_DIRECTORY -ChildPath $name; if (Test-Path -LiteralPath $path) { $found += $path } }; if ($found.Count -eq 0) { exit 0 }; Write-Host 'The portable package contains persistent state that will be permanently deleted:'; $found | ForEach-Object { Write-Host ('  ' + $_) }; $answer = Read-Host 'Delete it and rebuild the package? [y/N]'; if ($answer -match '^\s*(y|yes)\s*$') { exit 0 }; Write-Host 'Refusing to remove portable package; persistent state was kept.'; exit 1"
set "EXIT_CODE=%ERRORLEVEL%"
if not "%EXIT_CODE%"=="0" (
    echo Packaging cancelled.
    exit /b %EXIT_CODE%
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "if (Test-Path -LiteralPath '%PACKAGE_DIRECTORY%') { Remove-Item -LiteralPath '%PACKAGE_DIRECTORY%' -Recurse -Force }"
if not "%ERRORLEVEL%"=="0" (
    echo Could not remove the existing portable package.
    exit /b %ERRORLEVEL%
)

call "%~dp0build.bat" Release
set "EXIT_CODE=%ERRORLEVEL%"
if not "%EXIT_CODE%"=="0" (
    echo Paneacea package build failed. See the error above.
    exit /b %EXIT_CODE%
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "Compress-Archive -Path '%PACKAGE_DIRECTORY%\*' -DestinationPath '%ARCHIVE_PATH%' -Force"
set "EXIT_CODE=%ERRORLEVEL%"
if not "%EXIT_CODE%"=="0" (
    echo Could not create %ARCHIVE_PATH%.
    exit /b %EXIT_CODE%
)
echo Created %ARCHIVE_PATH%
exit /b 0
