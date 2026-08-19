SHELL := /bin/sh

GO ?= go
NPM ?= npm
DOCKER ?= docker
GOCACHE ?= /tmp/perpetual-gocache
BUILD_DIR ?= dist
BINARY := $(BUILD_DIR)/perpetual
BIN_DIR ?= $(HOME)/.local/bin
ARGS ?=

.PHONY: help fmt fmt-check test race vet web-install web-build web-test web-check go-build build check sandbox run install

help:
	@echo "Perpetual development commands"
	@echo ""
	@echo "  make sandbox     build the persistent workspace image"
	@echo "  make web-install install the locked React development dependencies"
	@echo "  make web-check   type-check, test, and build the React interface"
	@echo "  make run         start the local web application"
	@echo "  make test        run tests"
	@echo "  make check       format-check, test, race, vet, and build"
	@echo "  make build       build dist/perpetual"
	@echo "  make install     verify and install to $(BIN_DIR)/perpetual"

fmt:
	gofmt -w cmd internal

fmt-check:
	@test -z "$$(gofmt -l cmd internal)" || { \
		echo "Go files need formatting; run: make fmt"; \
		gofmt -l cmd internal; \
		exit 1; \
	}

test:
	GOCACHE=$(GOCACHE) $(GO) test -buildvcs=false ./...

race:
	GOCACHE=$(GOCACHE) $(GO) test -buildvcs=false -race ./...

vet:
	GOCACHE=$(GOCACHE) $(GO) vet -buildvcs=false ./...

web-install:
	cd web && $(NPM) ci

web-build:
	cd web && $(NPM) run build

web-test:
	cd web && $(NPM) test

web-check:
	cd web && $(NPM) run check

go-build:
	@mkdir -p $(BUILD_DIR)
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -o $(BINARY) ./cmd/perpetual

build: web-build go-build

check: fmt-check web-check test race vet go-build

sandbox:
	$(DOCKER) build -t perpetual-sandbox:dev sandbox

run: web-build
	GOCACHE=$(GOCACHE) $(GO) run -buildvcs=false ./cmd/perpetual $(ARGS)

install: check
	@install -d "$(BIN_DIR)"
	@install -m 0755 "$(BINARY)" "$(BIN_DIR)/perpetual"
	@echo "Installed $(BIN_DIR)/perpetual"
