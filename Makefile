.PHONY: install build build-frontend run run-server run-frontend lint diff-check \
	test test-cov check ci install-bin clean help

.DEFAULT_GOAL := help

BINARY_NAME := wt
BIN_DIR := bin
COVERAGE_FILE := coverage.out
FRONTEND_DIR := frontend

## install: Download Go dependencies and frontend npm packages
install:
	go mod download
	cd $(FRONTEND_DIR) && npm ci

## build-frontend: Build React SPA into internal/static/dist
build-frontend:
	cd $(FRONTEND_DIR) && npm run build
	@touch internal/static/dist/.gitkeep

## build: Build the monolithic binary (frontend embed + Go)
# ldflags inject buildinfo so wt web can show a "binary may be stale" badge
# when the source repo has advanced past the running binary's start time.
# See internal/handler/buildinfo.go.
build: build-frontend
	go build \
		-ldflags "\
			-X wt/internal/buildinfo.Commit=$$(git rev-parse HEAD 2>/dev/null) \
			-X wt/internal/buildinfo.CommitTime=$$(git log -1 --format=%ct HEAD 2>/dev/null) \
			-X wt/internal/buildinfo.SourceRepo=$$(pwd)" \
		-o $(BIN_DIR)/$(BINARY_NAME) .

## run: Run Go (-tags dev) + Vite in parallel for hot-reload development
run:
	$(MAKE) -j2 run-server run-frontend

## run-server: Run Go server with air hot-reload (-tags dev, listens on :8091)
run-server:
	air

## run-frontend: Run Vite dev server (proxies /api to the air backend on :8091)
run-frontend:
	cd $(FRONTEND_DIR) && npm run dev

## lint: Run golangci-lint
lint:
	golangci-lint run

## diff-check: Ensure no uncommitted changes to go.mod/go.sum
diff-check:
	go mod tidy
	git diff --exit-code go.mod go.sum

## test: Run Go tests
test:
	go test ./...

## test-cov: Run Go tests with coverage
test-cov:
	go test -coverprofile=$(COVERAGE_FILE) ./...
	go tool cover -func=$(COVERAGE_FILE)

## check: Run lint + test
check: lint test

## ci: Run lint + diff-check + test with coverage
ci: lint diff-check test-cov

## install-bin: Build and copy binary to ~/.config/wt/
install-bin: build
	cp $(BIN_DIR)/$(BINARY_NAME) $(HOME)/.config/wt/$(BINARY_NAME)

## clean: Remove build artifacts (keeps frontend node_modules)
clean:
	rm -rf $(BIN_DIR)/ $(COVERAGE_FILE)
	find internal/static/dist -mindepth 1 ! -name '.gitkeep' -delete 2>/dev/null || true

## help: Show this help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
