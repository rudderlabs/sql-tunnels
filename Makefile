default: help

GO          := go
GOLANG_CI	:= github.com/golangci/golangci-lint/cmd/golangci-lint@v1.57.1
GOFUMPT		:= mvdan.cc/gofumpt@latest

.PHONY: help
help: ## Show the available commands
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' ./Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: lint
lint: fmt ## Run linters on all go files
	$(GO) run $(GOLANGCI) run -v

.PHONY: fmt
fmt: ## Formats all go files
	$(GO) run $(GOFUMPT) -l -w -extra  .

.PHONY: test
test:
	$(GO) test ./... -race -count=1

.PHONY: coverage
coverage:
	$(GO) test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...