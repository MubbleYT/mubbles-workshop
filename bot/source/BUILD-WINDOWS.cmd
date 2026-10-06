@echo off
cd /d "%~dp0"
where go >nul 2>nul
if errorlevel 1 (
  echo Install Go from https://go.dev/dl/ to rebuild from source.
  pause
  exit /b 1
)
go test ./...
if errorlevel 1 goto failed
set "GOOS=windows"
set "GOARCH=amd64"
set "CGO_ENABLED=0"
go build -buildvcs=false -trimpath -ldflags="-s -w" -o MubbleDiscordBot.exe .
if errorlevel 1 goto failed
echo Built MubbleDiscordBot.exe. Double-click it to start.
pause
exit /b 0
:failed
echo Build failed. See the message above.
pause
exit /b 1
