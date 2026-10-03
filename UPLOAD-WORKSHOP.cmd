@echo off
cd /d "%~dp0"
where node >nul 2>nul
if errorlevel 1 (
  echo Install Node.js LTS from https://nodejs.org/ first, then reopen this window.
  pause
  exit /b 1
)
node tools/upload-source.mjs
pause
