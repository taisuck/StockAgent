@echo off
rem Linux용 바이너리를 output\stockagent 로 빌드한다.
setlocal
cd /d "%~dp0.."
if not exist output mkdir output
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -o output\stockagent .\src
if errorlevel 1 exit /b 1
echo built: output\stockagent
endlocal
