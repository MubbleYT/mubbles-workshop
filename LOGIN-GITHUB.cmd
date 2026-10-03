@echo off
where gh >nul 2>nul
if errorlevel 1 (
  echo Install GitHub CLI from https://cli.github.com/ first, then reopen this window.
  pause
  exit /b 1
)
gh auth login --hostname github.com --web
pause
