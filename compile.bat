set GOARCH=amd64

set GOOS=windows
go build -ldflags="-s -w" -o bin/updater.exe main.go

set GOOS=linux
go build -ldflags="-s -w" -o bin/updater-linux main.go

set GOOS=darwin
go build -ldflags="-s -w" -o bin/updater-mac main.go

set GOARCH=arm64
go build -ldflags="-s -w" -o bin/updater-mac-arm main.go
