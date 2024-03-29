default: help

.PHONY: help
help: ## Show the available commands
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' ./Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: install-tools
install-tools: ## install necessary tools
	go install mvdan.cc/gofumpt@latest
	bash ./scripts/install-golangci-lint.sh v1.55.2

.PHONY: lint
lint: fmt ## Run linters on all go files
	golangci-lint run -v --timeout 5m

.PHONY: fmt 
fmt: install-tools ## Formats all go files
	gofumpt -l -w -extra  .

.PHONY: test
test:
	go test ./... -race -count=1

.PHONY: coverage
coverage:
	go test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...