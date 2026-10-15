# oc-ops Makefile — build, install, test
#
# Builds the Go CLI binary and installs it into $PREFIX/bin, and links the
# opencode plugin into ~/.config/opencode/plugins.

PREFIX  ?= $(shell echo $${PREFIX:-/data/data/com.termux/files/usr})
BIN     ?= bin/oc-ops
GO      ?= go
VERSION ?= 0.1.0
LDFLAGS  = -ldflags "-X main.version=$(VERSION)"

.PHONY: all build install uninstall test vet clean plugin-install plugin-uninstall link plugin-test

all: build

## build: compile the Go binary
build:
	$(GO) build $(LDFLAGS) -o $(BIN) ./cmd/oc-ops

## test: run the Go test suite
test:
	$(GO) test ./...

## plugin-test: run the opencode plugin tests
plugin-test:
	cd plugin && node --test tools/test.mjs

## vet: static analysis
vet:
	$(GO) vet ./...

## install: build + install CLI into $PREFIX/bin + link the opencode plugin
install: build plugin-install
	install -m 0755 $(BIN) "$(PREFIX)/bin/oc-ops"
	@echo "installed $(PREFIX)/bin/oc-ops"

## plugin-install: link the opencode plugin into the global plugins dir
plugin-install:
	@mkdir -p "$(HOME)/.config/opencode/plugins"
	@ln -sfn "$(CURDIR)/plugin" "$(HOME)/.config/opencode/plugins/oc-ops"
	@echo "linked plugin -> $(HOME)/.config/opencode/plugins/oc-ops"

## uninstall: remove the installed CLI and the plugin link
uninstall: plugin-uninstall
	rm -f "$(PREFIX)/bin/oc-ops"

plugin-uninstall:
	rm -f "$(HOME)/.config/opencode/plugins/oc-ops"

## clean: remove build artifacts
clean:
	rm -f $(BIN)
	rm -rf "$(HOME)/.cache/oc-ops"
