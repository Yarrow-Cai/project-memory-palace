.PHONY: all build build-cli build-mcp build-gui test clean

all: build

build: build-cli build-mcp

build-cli:
	go build -o bin/pmem.exe ./cmd/pmem

build-mcp:
	go build -o bin/pmem-mcp.exe ./cmd/pmem-mcp

build-gui:
	go build -o bin/pmem-gui.exe ./cmd/pmem-gui

test:
	go test ./internal/... -v -count=1

clean:
	rm -rf bin/
