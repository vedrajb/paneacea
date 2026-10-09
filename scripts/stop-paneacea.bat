@echo off
setlocal EnableExtensions
rem Stops only the paneacea.exe running from the paneacea-portable folder.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0stop-paneacea.ps1"
exit /b %ERRORLEVEL%
