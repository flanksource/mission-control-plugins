OS   = $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH = $(shell uname -m | sed 's/x86_64/amd64/')
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DATE ?= $(shell date "+%Y-%m-%d %H:%M:%S")
PLUGIN_PATH ?= $(HOME)/.mission-control/plugins

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/.bin
export PATH := $(LOCALBIN):$(PATH)
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
DEPS ?= $(LOCALBIN)/deps
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint

## Tool Versions
GOLANGCI_LINT_VERSION ?= 2.12.2

PLUGIN_MODULES := $(sort $(dir $(shell find . -mindepth 2 -maxdepth 2 -name go.mod)))
PLUGINS := $(patsubst %/,%,$(PLUGIN_MODULES))

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: install-deps
install-deps: $(DEPS) ## Install the deps CLI if not present

$(DEPS): | $(LOCALBIN)
	@if [ -x "$(DEPS)" ]; then \
		exit 0; \
	elif command -v deps >/dev/null 2>&1; then \
		ln -sf "$$(command -v deps)" "$(DEPS)"; \
	else \
		curl -sSL https://github.com/flanksource/deps/releases/latest/download/deps-$(OS)-$(ARCH).tar.gz | tar -xz -C $(LOCALBIN); \
	fi

.PHONY: deps
deps: install-deps golangci-lint ## Install all tool dependencies

.PHONY: golangci-lint
golangci-lint: install-deps $(LOCALBIN) ## Install golangci-lint locally
	$(DEPS) install golangci/golangci-lint@v$(GOLANGCI_LINT_VERSION) --bin-dir $(LOCALBIN)

.PHONY: pnpm
pnpm: ## Verify global pnpm is available
	@command -v pnpm >/dev/null 2>&1 || (echo "pnpm is required on PATH" >&2; exit 1)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint on all plugin modules
	@set -e; \
	for module in $(PLUGIN_MODULES); do \
		echo "Running golangci-lint in $$module"; \
		(cd $$module && $(GOLANGCI_LINT) run --path-mode=abs); \
	done

.PHONY: plugins-ui
plugins-ui: pnpm ## Build all plugin UI bundles
	@set -e; \
	for plugin in $(PLUGINS); do \
		if [ -f "$$plugin/ui-src/package.json" ]; then \
			echo "Building UI for $$plugin"; \
			(cd "$$plugin/ui-src" && CI=true pnpm install --no-frozen-lockfile --prefer-offline && PLUGIN_VERSION="$(VERSION)" PLUGIN_BUILD_DATE="$(BUILD_DATE)" pnpm run build); \
		fi; \
	done

.PHONY: generate-ui-checksums
generate-ui-checksums: plugins-ui ## Regenerate embedded UI checksums for all plugins
	@set -e; \
	for plugin in $(PLUGINS); do \
		echo "Regenerating UI checksum for $$plugin"; \
		(cd "$$plugin" && go generate ./...); \
	done

.PHONY: build
build: generate-ui-checksums ## Build all plugin UIs and Go binaries into PLUGIN_PATH (default: ~/.mission-control/plugins)
	@set -e; \
	mkdir -p "$(PLUGIN_PATH)"; \
	for plugin in $(PLUGINS); do \
		name=$$(basename "$$plugin"); \
		echo "Building $$name -> $(PLUGIN_PATH)/$$name"; \
		(cd "$$plugin" && go build -o "$(PLUGIN_PATH)/$$name" -ldflags "-X 'main.Version=$(VERSION)' -X 'main.BuildDate=$(BUILD_DATE)'" .); \
	done
