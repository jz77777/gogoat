set GOARCH=amd64

set GOOS=windows
go build -ldflags="-s -w" -o bin/updater.exe gogoat.go

set GOOS=linux
go build -ldflags="-s -w" -o bin/updater-linux gogoat.go

set GOOS=darwin
go build -ldflags="-s -w" -o bin/updater-mac gogoat.go

set GOARCH=arm64
go build -ldflags="-s -w" -o bin/updater-mac-arm gogoat.go
