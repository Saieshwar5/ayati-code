SHELL := /bin/sh

GO ?= go
DOCKER ?= docker
GOCACHE ?= /tmp/ayati-code-gocache
BUILD_DIR ?= dist
BINARY := $(BUILD_DIR)/ayati
BIN_DIR ?= $(HOME)/.local/bin
ARGS ?=

.PHONY: help fmt fmt-check test race vet build check sandbox run install

help:
	@echo "Ayati development commands"
	@echo ""
	@echo "  make sandbox     build the persistent workspace image"
	@echo "  make run         start the local web application"
	@echo "  make test        run tests"
	@echo "  make check       format-check, test, race, vet, and build"
	@echo "  make build       build dist/ayati"
	@echo "  make install     verify and install to $(BIN_DIR)/ayati"

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

build:
	@mkdir -p $(BUILD_DIR)
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -o $(BINARY) ./cmd/ayati

check: fmt-check test race vet build

sandbox:
	$(DOCKER) build -t ayati-sandbox:dev sandbox

run:
	GOCACHE=$(GOCACHE) $(GO) run -buildvcs=false ./cmd/ayati $(ARGS)

install: check
	@install -d "$(BIN_DIR)"
	@install -m 0755 "$(BINARY)" "$(BIN_DIR)/ayati"
	@echo "Installed $(BIN_DIR)/ayati"
