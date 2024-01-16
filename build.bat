@echo off
rem 判断是直接运行还是cmd /c调用
if "%~1"=="" (
    goto :INIT
) else (
    goto :BUILD && exit
)
:INIT
echo init script
cmd /c "%~f0 1234"
rem 在新的cmd运行自身防止污染本地环境变量，加个参数好判断
goto :EOF
:BUILD
cd /d "%~dp0"
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
