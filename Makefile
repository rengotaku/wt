.PHONY: install build lint diff-check test test-cov check ci install-bin clean help

.DEFAULT_GOAL := help

BINARY_NAME := wt
BIN_DIR := bin
COVERAGE_FILE := coverage.out

## install: Download Go dependencies
install:
	go mod download

## build: Build the binary
build:
	go build -o $(BIN_DIR)/$(BINARY_NAME) .

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

## install-bin: Build and install binary to ~/.config/wt/
# 同一パスへの cp で上書きすると、macOS が持つ実行ファイルの署名キャッシュが
# 食い違い、以後そのパスの実行が起動直後に SIGKILL される。rename は新しい
# inode になるため起きないので、一時ファイルへ置いてから mv で差し替える。
install-bin: build
	cp $(BIN_DIR)/$(BINARY_NAME) $(HOME)/.config/wt/$(BINARY_NAME).new
	mv -f $(HOME)/.config/wt/$(BINARY_NAME).new $(HOME)/.config/wt/$(BINARY_NAME)

## clean: Remove build artifacts
clean:
	rm -rf $(BIN_DIR)/ $(COVERAGE_FILE)

## help: Show this help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
