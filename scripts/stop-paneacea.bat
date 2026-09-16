@echo off
setlocal EnableExtensions

set "STOPPED_COUNT=0"
set "FAILURE=0"

for %%P in (paneacea.exe paneacea-runtime.exe paneacea-cli.exe) do (
    tasklist /FI "IMAGENAME eq %%P" /NH | findstr /I /C:"%%P" >nul
    if not errorlevel 1 (
        echo Closing %%P...
        taskkill /F /IM "%%P" >nul
        if errorlevel 1 (
            echo Failed to stop %%P.
            set "FAILURE=1"
        ) else (
            set /A STOPPED_COUNT+=1
        )
    )
)

if "%STOPPED_COUNT%"=="0" echo No Paneacea processes are running.
if not "%FAILURE%"=="0" exit /b 1

if not "%STOPPED_COUNT%"=="0" echo Stopped all found Paneacea processes.
exit /b 0
