.PHONY: build clean test

build:
    if not exist bin mkdir bin
    go build -o bin/provisioner.exe ./cmd/provisioner
    go build -o bin/reconciler.exe ./cmd/reconciler
    go build -o bin/simulator.exe ./cmd/simulator
    go build -o bin/starter.exe ./cmd/starter
    go build -o bin/cleaner.exe ./cmd/cleaner

clean:
    if exist bin rmdir /s /q bin

test:
    go test ./...