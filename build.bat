@echo off
if "%~1"=="" (
    goto :INIT
) else (
    goto :BUILD && exit
)
:INIT
echo init script
cmd /c "%~f0 1234"
goto :EOF
:BUILD
echo Start Build
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -ldflags "-s" -o releases/portscan.exe
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -ldflags "-s -d" -o releases/portscan
:EOF
echo End
