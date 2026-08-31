.PHONY: build test install uninstall run release-check

build:
	go build -o bin/hey-claudex ./cmd/hey-claudex

test:
	go test ./...

install: build
	./bin/hey-claudex install

uninstall:
	hey-claudex uninstall

run:
	hey-claudex

release-check: test build
	git diff --check
