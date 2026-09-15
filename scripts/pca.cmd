@echo off
setlocal
start "" /B /D "%~dp0exes" "%~dp0exes\paneacea.exe" %*
exit /b %ERRORLEVEL%
