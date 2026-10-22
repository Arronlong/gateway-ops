@echo off
setlocal EnableExtensions
chcp 65001 >nul
set "SCRIPT=%~dp0tools\restart-test.ps1"
if not exist "%SCRIPT%" (
    echo Cannot find the test launcher: %SCRIPT%
    exit /b 1
)
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT%" %*
exit /b %ERRORLEVEL%
