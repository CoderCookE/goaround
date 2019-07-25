APP_NAME=goaround
OS=linux
ARCH=amd64
PKG_NAME=$(APP_NAME)_$(shell cat VERSION)_$(ARCH)
RELEASE=$$(git rev-parse HEAD)

default: bin

bin:
	mkdir -p bin
	go build -o ./bin/$(APP_NAME)

test:
	go test -race -v ./...

.PHONY: bin default
