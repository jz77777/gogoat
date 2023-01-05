set GOARCH=amd64

set GOOS=windows
go build -o bin/gogoat.exe gogoat.go

set GOOS=linux
go build -o bin/gogoat-linux gogoat.go

set GOOS=darwin
go build -o bin/gogoat-mac gogoat.go

set GOARCH=arm64
go build -o bin/gogoat-mac-arm gogoat.go
