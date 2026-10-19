.DEFAULT_GOAL := help

COMPOSE := docker compose -p garage-webui-dev -f dev/compose/compose.yaml
OIDC_COMPOSE := docker compose -p garage-webui-oidc-test -f dev/compose/compose.oidc.yaml
ROUTE_PREFIX ?=

# renovate: datasource=github-releases depName=kumbuka-me/cli
KUMBUKA_CLI_VERSION ?= v0.12.0
# renovate: datasource=github-releases depName=gi8lino/dev-tools
DEV_TOOLS_VERSION ?= v0.9.0

# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.15.0
GOLANGCI_LINT := bin/golangci-lint

include bin/dev-tools.mk
include $(call dev-tools-module,tag)
include $(call dev-tools-module,port)
include $(call dev-tools-module,browser)
include $(call dev-tools-module,help)

KUMBUKA_CLI := bin/kumbuka-cli-$(KUMBUKA_CLI_VERSION)
KUMBUKA_CLI_ASSET ?= kumbuka-cli_{version}_{os}_{arch}.tar.gz
SITE_CONFIG ?= docs/site.toml
SITE_OUTPUT := docs/site
SITE_PORT = $(call dev-port,site)
SITE_URL = http://127.0.0.1:$(SITE_PORT)/
PYTHON ?= python3
FAVICON_GENERATE := $(DEV_TOOLS_BIN)/favicon-generate
FAVICON_SOURCE ?= web/public/logo.svg
FAVICON_SIZES ?= 16x16 32x32

$(FAVICON_GENERATE): | $(DEV_TOOLS_BIN)
	$(call download-dev-tool,favicon-generate,$@)

##@ Documentation

.PHONY: site site-serve screenshots kumbuka dev-tools
screenshots: ## Capture documentation screenshots using a disposable local stack.
	sh dev/capture-screenshots.sh

site: kumbuka ## Build the documentation site.
	$(KUMBUKA_CLI) build --config "$(SITE_CONFIG)"

site-serve: kumbuka $(DEV_PORT) $(OPEN_BROWSER) ## Build, serve, and open the documentation locally.
	$(KUMBUKA_CLI) build --config "$(SITE_CONFIG)" --site-url "$(SITE_URL)"
	@echo "Serving Garage Web UI documentation at $(SITE_URL)"
	@$(OPEN_BROWSER) "$(SITE_URL)" & \
		browser_pid=$$!; trap 'kill "$$browser_pid" 2>/dev/null || true' EXIT; \
		$(PYTHON) -m http.server "$(SITE_PORT)" --bind 127.0.0.1 --directory "$(SITE_OUTPUT)"

kumbuka: $(KUMBUKA_CLI) ## Install the pinned Kumbuka CLI.

$(KUMBUKA_CLI): $(GITHUB_RELEASE_INSTALL)
	@$(GITHUB_RELEASE_INSTALL) --repo kumbuka-me/cli --tag "$(KUMBUKA_CLI_VERSION)" \
		--asset "$(KUMBUKA_CLI_ASSET)" --binary kumbuka-cli --target "$@"

dev-tools: $(DEV_TAG) $(DEV_PORT) $(OPEN_BROWSER) $(MAKE_HELP) $(GITHUB_RELEASE_INSTALL) $(FAVICON_GENERATE) ## Download pinned development tools.

##@ Assets

.PHONY: favicon
favicon: $(FAVICON_GENERATE) $(FAVICON_SOURCE) ## Generate Web UI and documentation favicons from the logo.
	$(call run-tool,$(FAVICON_GENERATE),--apple-touch "$(FAVICON_SOURCE)" "web/public" $(FAVICON_SIZES))
	$(call run-tool,$(FAVICON_GENERATE),--apple-touch "$(FAVICON_SOURCE)" "docs/content/assets" $(FAVICON_SIZES))

##@ Development

frontend: ## Install frontend dependencies and build the frontend.
	cd web && npm ci && npm run build

build: frontend ## Build the frontend and application binary.
	go build -o bin/garage-webui ./cmd/garage-webui

# Garage and Postgres need no manual S3 initialization: the Web UI provisions
# a bucket-scoped signing key and CORS automatically on first upload.
dev-up: ## Start Garage and PostgreSQL for local development.
	$(COMPOSE) up -d --wait garage postgres
	$(COMPOSE) run --rm --no-deps garage-ready

oidc-provider-up: ## Start the disposable local Keycloak test provider.
	$(OIDC_COMPOSE) up -d

oidc-provider-down: ## Stop and remove the disposable Keycloak test provider and its ephemeral data.
	$(OIDC_COMPOSE) down

run: dev-up frontend ## Run the application locally.
	GARAGE_WEBUI__DATABASE_URL='postgres://garage_webui:local-database-password@localhost:5432/garage_webui?sslmode=disable' \
	GARAGE_WEBUI__GARAGE_URL='http://localhost:3903' \
	GARAGE_WEBUI__GARAGE_TOKEN='local-garage-admin-token' \
	GARAGE_WEBUI__AUTH_MODE='local' \
	GARAGE_WEBUI__PUBLIC_URL='http://localhost:8080' \
	GARAGE_WEBUI__S3_PUBLIC_URL='http://localhost:3900' \
	GARAGE_WEBUI__ROUTE_PREFIX='$(ROUTE_PREFIX)' \
	go run ./cmd/garage-webui --debug --access-log

test: frontend ## Run frontend and Go tests and static analysis.
	cd web && npm test
	go test ./...
	go vet ./...

test-e2e: ## Build, run all E2E tests in an isolated stack, then clean it up.
	sh dev/run-e2e.sh

test-e2e-existing: ## Run browser tests against a separately managed isolated application (set E2E_BASE_URL).
	cd web && npm run test:e2e

fmt: fmt-go fmt-web fmt-md ## Format Go, frontend, and Markdown files.

fmt-go: ## Format Go files.
	gofmt -w cmd internal themes web

fmt-web: ## Format frontend files.
	cd web && npm exec --no -- prettier --write "src/**/*.{ts,tsx,css}" "public/*.css" "public/*.html" "index.html" "vite.config.ts"

fmt-md: ## Format the README.
	cd web && npm exec --no -- prettier --write --prose-wrap never ../README.md ../docs/**/*.md

check-format: ## Check Go formatting and patch whitespace.
	@unformatted="$$(gofmt -l cmd internal themes web)"; \
	if [ -n "$$unformatted" ]; then \
		printf 'Go files need formatting:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi
	git diff --check

clean: ## Clean Go build cache.
	go clean

.PHONY: lint
lint: lint-go ## Run all linters and formatting checks.

.PHONY: lint-go
lint-go: golangci-lint ## Run golangci-lint.
	$(GOLANGCI_LINT) run

##@ Dependencies

.PHONY: golangci-lint
golangci-lint: $(GO_INSTALL_TOOL) ## Download golangci-lint locally if necessary.
	@$(GO_INSTALL_TOOL) \
		--target "$(GOLANGCI_LINT)" \
		--package github.com/golangci/golangci-lint/v2/cmd/golangci-lint \
		--tool-version "$(GOLANGCI_LINT_VERSION)"


