@echo off
setlocal EnableExtensions

where.exe powershell.exe >nul 2>&1
if errorlevel 1 (
    echo cx installer: powershell.exe is required. 1>&2
    exit /b 1
)

powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference = 'Stop'; $script = (Invoke-WebRequest -UseBasicParsing -Uri 'https://raw.githubusercontent.com/masahide/codex-profile-switcher/main/install.ps1').Content; & ([scriptblock]::Create($script))"
set "CX_INSTALLER_EXIT=%ERRORLEVEL%"

endlocal & exit /b %CX_INSTALLER_EXIT%
