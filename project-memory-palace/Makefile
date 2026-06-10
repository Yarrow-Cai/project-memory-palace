.PHONY: all build clean test

all: build

build:
	go build -o bin/pmem.exe ./cmd/pmem

test:
	go test ./internal/... -count=1

clean:
	rm -rf bin/pmem.exe
