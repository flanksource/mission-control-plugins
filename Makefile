OS   = $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH = $(shell uname -m | sed 's/x86_64/amd64/')

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

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: install-deps
install-deps: $(LOCALBIN) ## Install the deps CLI if not present
	which deps 2>/dev/null || test -x $(DEPS) || curl -sSL https://github.com/flanksource/deps/releases/latest/download/deps-$(OS)-$(ARCH).tar.gz | tar -xz -C $(LOCALBIN)

.PHONY: deps
deps: install-deps golangci-lint ## Install all tool dependencies

.PHONY: golangci-lint
golangci-lint: install-deps $(LOCALBIN) ## Install golangci-lint locally
	$(DEPS) install golangci/golangci-lint@v$(GOLANGCI_LINT_VERSION) --bin-dir $(LOCALBIN)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint on all plugin modules
	@set -e; \
	for module in $(PLUGIN_MODULES); do \
		echo "Running golangci-lint in $$module"; \
		(cd $$module && $(GOLANGCI_LINT) run --path-mode=abs); \
	done
