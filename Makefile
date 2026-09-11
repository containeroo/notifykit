.DEFAULT_GOAL := help

## Tool Versions

# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.13.2

# renovate: datasource=github-releases depName=gi8lino/dev-tools
DEV_TOOLS_VERSION ?= v0.7.0

# renovate: datasource=github-releases depName=gi8lino/lore
LORE_VERSION ?= v0.13.0

# renovate: datasource=npm depName=prettier
PRETTIER_VERSION ?= 3.9.6


## Shared Development Tools

include bin/dev-tools.mk
include $(call dev-tools-module,tag)
include $(call dev-tools-module,port)
include $(call dev-tools-module,browser)
include $(call dev-tools-module,help)


## Project Tools

GOLANGCI_LINT := bin/golangci-lint

LORE := bin/lore
LORE_ASSET ?= lore_{version}_{os}_{arch}.tar.gz


## Documentation

SITE_CONFIG ?= docs/site.toml
SITE_PORT ?= $(call dev-port,site)
SITE_URL = http://127.0.0.1:$(SITE_PORT)/


## Formatting

NPX ?= npx
PRETTIER_MD_SOURCES := README.md "docs/content/**/*.md"


##@ Tagging

.PHONY: tag
tag: current


##@ Development

.PHONY: fmt-md
fmt-md: ## Format Markdown files with Prettier.
	$(NPX) --yes prettier@$(PRETTIER_VERSION) --write $(PRETTIER_MD_SOURCES)

.PHONY: fmt
fmt: fmt-md ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet ## Run unit tests.
	go test -race -covermode=atomic -count=1 -parallel=4 -timeout=5m ./...

.PHONY: cover
cover: ## Display test coverage.
	go test -race -coverprofile=coverage.out -covermode=atomic -count=1 -parallel=4 -timeout=5m ./...
	go tool cover -html=coverage.out

.PHONY: clean
clean: ## Clean up generated files.
	find . -type f -name '*.out' -delete


##@ Documentation

.PHONY: lore
lore: $(GITHUB_RELEASE_INSTALL) ## Install the pinned Lore release.
	@$(GITHUB_RELEASE_INSTALL) \
		--repo gi8lino/lore \
		--tag "$(LORE_VERSION)" \
		--asset "$(LORE_ASSET)" \
		--binary lore \
		--target "$(LORE)"

.PHONY: site
site: lore ## Build the published read-only documentation site.
	$(LORE) build --config "$(SITE_CONFIG)"

.PHONY: site-open
site-open: $(DEV_PORT) $(OPEN_BROWSER) ## Open the documentation site once it responds.
	$(call run-tool,$(OPEN_BROWSER),"$(SITE_URL)")

.PHONY: site-serve
site-serve: lore $(DEV_PORT) $(OPEN_BROWSER) ## Build, serve, and open the documentation site locally.
	$(LORE) build \
		--config "$(SITE_CONFIG)" \
		--site-url "$(SITE_URL)"
	@echo "Serving documentation at $(SITE_URL)"
	@$(OPEN_BROWSER) "$(SITE_URL)" & \
	browser_pid=$$!; \
	trap 'kill "$$browser_pid" 2>/dev/null || true' EXIT; \
	python3 -m http.server $(SITE_PORT) \
		--bind 127.0.0.1 \
		--directory docs/site


##@ Dependencies

.PHONY: golangci-lint
golangci-lint: $(GO_INSTALL_TOOL) ## Download golangci-lint locally if necessary.
	@$(GO_INSTALL_TOOL) \
		--target "$(GOLANGCI_LINT)" \
		--package github.com/golangci/golangci-lint/v2/cmd/golangci-lint \
		--tool-version "$(GOLANGCI_LINT_VERSION)"
