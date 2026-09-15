@echo off
setlocal

if /I "%~1"=="--help" (
    echo Usage: package.bat
    echo Deletes the existing portable package, rebuilds it, and creates portable-release.zip.
    exit /b 0
)

if not "%~1"=="" (
    echo Unknown option: "%~1"
    echo Usage: package.bat
    exit /b 1
)

set "PACKAGE_DIRECTORY=%~dp0portable-release"
set "ARCHIVE_PATH=%~dp0portable-release.zip"
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
