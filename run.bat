@echo off
setlocal

if /I "%~1"=="--help" (
    echo Usage: run.bat [Debug^|Release]
    echo Builds the portable package and launches Paneacea.
    echo The default configuration is Release.
    exit /b 0
)

if not "%~1"=="" if /I not "%~1"=="Debug" if /I not "%~1"=="Release" (
    echo Unknown option: "%~1"
    echo Usage: run.bat [Debug^|Release]
    exit /b 1
)

set "CONFIGURATION=%~1"
if "%CONFIGURATION%"=="" set "CONFIGURATION=Release"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\build-wails.ps1" -Configuration "%CONFIGURATION%" -Run

set "EXIT_CODE=%ERRORLEVEL%"
if not "%EXIT_CODE%"=="0" (
    echo Paneacea build and run failed. See the error above.
    if "%~1"=="" pause
)
exit /b %EXIT_CODE%
