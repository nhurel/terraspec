SHELL := /bin/bash
BINARY=terraspec

SOURCES := $(shell find . -name '*.go')

git_tag = $(shell git describe --tags --long | sed -e 's/-/./g' | awk -F '.' '{print $$1"."$$2"."$$3+$$4}')

.PHONY: clean install vet lint fmt current_version version_bump

default: build

all: clean fmt vet test $(BINARY) install

$(BINARY): $(SOURCES)
	CGO_ENABLED=0 go build -a -installsuffix cgo -o $(BINARY) -ldflags "-s -X main.Version=$(git_tag)" .

distrib:
	rm -rf dist && mkdir -p dist
	docker run --rm -v "$$PWD":/go/src/github.com/nhurel/terraspec -w /go/src/github.com/nhurel/terraspec -e GOOS=windows -e GOARCH=amd64 golang:1.14.4 go build -o dist/$(BINARY)-windows.exe -ldflags "-s -X main.Version=$(git_tag)"
	docker run --rm -v "$$PWD":/go/src/github.com/nhurel/terraspec -w /go/src/github.com/nhurel/terraspec -e GOOS=linux -e GOARCH=amd64 golang:1.14.4 go build -o dist/$(BINARY)-linux-x64 -ldflags "-s -X main.Version=$(git_tag)"
	docker run --rm -v "$$PWD":/go/src/github.com/nhurel/terraspec -w /go/src/github.com/nhurel/terraspec -e GOOS=darwin -e GOARCH=amd64 golang:1.14.4 go build -o dist/$(BINARY)-darwin -ldflags "-s -X main.Version=$(git_tag)"

install: $(BINARY)
	CGO_ENABLED=0 go install -installsuffix cgo -ldflags "-s -X main.Version=$(git_tag)"

clean:
	go clean

test: fmt
	go test ./...

vet: fmt
	go vet ./...

fmt:
	go fmt ./...

current_version:
	@echo $(git_tag)

version_bump:
	git pull --tags
	n=$$(git describe --tags --long | sed -e 's/-/./g' | awk -F '.' '{print $$4}'); \
	maj=$$(git log --format=oneline -n $$n | grep "#major"); \
	min=$$(git log --format=oneline -n $$n | grep "#minor"); \
	if [ -n "$$maj" ]; then \
		TAG=$(shell git describe --tags --long | sed -e 's/-/./g' | awk -F '.' '{print $$1+1".0.0"}'); \
	elif [ -n "$$min" ]; then \
		TAG=$(shell git describe --tags --long | sed -e 's/-/./g' | awk -F '.' '{print $$1"."$$2+1".0"}'); \
	else \
		TAG=$(shell git describe --tags --long | sed -e 's/-/./g' | awk -F '.' '{print $$1"."$$2"."$$3+$$4+1}'); \
	fi; \
	git tag -a -m "Automatic version bump" $$TAG
	git push --tags

