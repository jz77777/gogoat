set GOARCH=amd64

set GOOS=windows
go build -o gogoat.exe gogoat.go

set GOOS=darwin
go build -o gogoat-mac gogoat.go

set GOOS=linux
go build -o gogoat-lin gogoat.go
