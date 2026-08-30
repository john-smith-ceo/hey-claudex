.PHONY: build test install uninstall run release-check

build:
	go build -o bin/hey-codex ./cmd/hey-codex

test:
	go test ./...

install: build
	./bin/hey-codex install

uninstall:
	hey-codex uninstall

run:
	hey-codex

release-check: test build
	git diff --check
