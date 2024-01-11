@echo off
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -o releases/portscan.exe
SET CGO_ENABLED=0
SET GOOS=linux
SET GOARCH=amd64
go build -ldflags="-s -d" -o releases/portscan
