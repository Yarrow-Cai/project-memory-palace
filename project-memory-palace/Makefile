.PHONY: all build clean test

all: build

build: build-cli build-mcp build-desktop

build-cli:
	go build -o bin/pmem.exe ./cmd/pmem

build-mcp:
	go build -o bin/pmem-mcp.exe ./cmd/pmem-mcp

build-desktop:
	go build -ldflags "-H windowsgui" -o bin/pmem-tray.exe ./cmd/pmem-tray

test:
	go test ./internal/... -count=1

clean:
	rm -rf bin/pmem.exe bin/pmem-mcp.exe bin/pmem-tray.exe
