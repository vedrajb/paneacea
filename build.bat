@echo off
setlocal

if /I "%~1"=="--help" (
    echo Usage: build-wails.bat [--build-only]
    echo Closes this project's Paneacea apps and terminal sessions, then builds and launches.
    echo Use --build-only to build without launching.
    exit /b 0
)

if not "%~1"=="" if /I not "%~1"=="--build-only" (
    echo Unknown option: "%~1"
    echo Usage: build-wails.bat [--build-only]
    exit /b 1
)

if /I "%~1"=="--build-only" (
    powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\build-wails.ps1"
) else (
    powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\build-wails.ps1" -Run
)

set "EXIT_CODE=%ERRORLEVEL%"
if not "%EXIT_CODE%"=="0" (
    echo Paneacea build failed. See the error above.
    if "%~1"=="" pause
)
exit /b %EXIT_CODE%
