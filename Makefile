# oc-ops Makefile — build, install, test
#
# Builds the Go CLI binary and installs it into $PREFIX/bin, and registers the
# opencode plugin (v1 @opencode-ai/plugin and v2 @opencode/plugin) so either
# opencode generation loads it.

# Termux sets `$PREFIX`; on other Unix-like systems default to /usr/local
# (override with `make install PREFIX=~/.local` when root is unavailable).
PREFIX  ?= $(shell if [ -n "$$PREFIX" ]; then printf '%s' "$$PREFIX"; elif [ -d /data/data/com.termux ]; then printf '%s' /data/data/com.termux/files/usr; else printf '%s' /usr/local; fi)
BIN     ?= bin/oc-ops
GO      ?= go
VERSION ?= 0.1.0
LDFLAGS  = -ldflags "-X main.version=$(VERSION)"

# Fall back to common install locations when `go` is not on PATH
# (e.g. WSL2/Debian with the toolchain at /opt/go1.x/bin or /usr/local/go/bin).
ifeq ($(shell command -v $(GO) 2>/dev/null),)
GO := $(shell ls -d /opt/go*/bin/go /usr/local/go/bin/go 2>/dev/null | head -1)
endif

.PHONY: all build test vet clean install uninstall plugin-install plugin-uninstall link plugin-test

all: build

## build: compile the Go binary
build:
	@if [ -z "$(GO)" ]; then echo "error: Go toolchain not found (go 1.27+ required); set GO=/path/to/go" >&2; exit 1; fi
	$(GO) build $(LDFLAGS) -o $(BIN) ./cmd/oc-ops

## test: run the Go test suite
test:
	$(GO) test ./...

## vet: static analysis
vet:
	$(GO) vet ./...

## plugin-test: run the opencode plugin tests
plugin-test:
	cd plugin && node --test tools/test.mjs

## install: build + install CLI into $PREFIX/bin + register the opencode plugin
install: build plugin-install
	mkdir -p "$(PREFIX)/bin"
	install -m 0755 $(BIN) "$(PREFIX)/bin/oc-ops"
	@echo "installed $(PREFIX)/bin/oc-ops"

## plugin-install: register the plugin in the global opencode.json
## Registers both the v1 plugin (opencode `plugin` array) and the v2 plugin
## (opencode `plugins` array) with the installed binary path.
plugin-install:
	@node scripts/plugin-register.js install "$(PREFIX)/bin/oc-ops"

## uninstall: remove the installed CLI and the plugin registration
uninstall: plugin-uninstall
	rm -f "$(PREFIX)/bin/oc-ops"

plugin-uninstall:
	@node scripts/plugin-register.js uninstall

## clean: remove build artifacts
clean:
	rm -f $(BIN)
	rm -rf "$(HOME)/.cache/oc-ops"