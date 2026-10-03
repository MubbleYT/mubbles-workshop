@echo off
cd /d "%~dp0"
echo Starting Mubble's Workshop...
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-windows.ps1"
if errorlevel 1 pause
