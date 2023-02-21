set GOARCH=amd64

set GOOS=windows
go build -ldflags="-s -w" -o bin/gogoat.exe gogoat.go

set GOOS=linux
go build -ldflags="-s -w" -o bin/gogoat-linux gogoat.go

set GOOS=darwin
go build -ldflags="-s -w" -o bin/gogoat-mac gogoat.go

set GOARCH=arm64
go build -ldflags="-s -w" -o bin/gogoat-mac-arm gogoat.go
