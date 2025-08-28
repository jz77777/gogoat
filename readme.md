# gogoat

Forked from enu's open-source repo of the same name.

A repo for building `updater` executables for Pokemon Rejuvenation - in this case, I've used it for the Overseer Files mod.

## Running locally

```bash
brew install go
chmod +x ./main.go
# go run ./main.go  won't work since it requires external files (updater.yaml) so instead:
go build # this produces an executable called "gogoat"
./gogoat
```