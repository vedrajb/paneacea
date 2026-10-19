@echo off
setlocal
rem pca: open Paneacea without waiting; pca ARGS: run a CLI command and print its result.
if "%~1"=="" (
    start "" /D "%~dp0" "%~dp0paneacea.exe"
    exit /b %ERRORLEVEL%
)
"%~dp0paneacea-cli.exe" %*
exit /b %ERRORLEVEL%
