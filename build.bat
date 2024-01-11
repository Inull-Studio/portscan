@echo off
go env CGO_ENABLED=0
go env GOOS=windows
go env GOARCH=amd64
go build -o releases/portscan.exe
go env CGO_ENABLED=0
go env GOOS=linux
go env GOARCH=amd64
go build -ldflags="-s -d" -o releases/portscan
