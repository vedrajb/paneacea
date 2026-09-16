@echo off
setlocal
start "" /B /D "%~dp0" "%~dp0paneacea.exe" %*
exit /b %ERRORLEVEL%
