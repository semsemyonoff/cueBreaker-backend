# cueBreaker backend — local dev tasks.
#
# The single binary embeds the built SPA from web/dist (a placeholder in this
# repo; the real SPA is baked in at image-build time).

APP_VERSION ?= dev
BIN         ?= cuebreaker

.PHONY: build run test test-race cover vet lint fmt tidy clean

build: ## Build the cuebreaker binary (SPA embedded from web/dist)
	go build -ldflags "-X main.version=$(APP_VERSION)" -o $(BIN) ./cmd/cuebreaker

run: ## Run the server (go run)
	go run ./cmd/cuebreaker

test: ## Run all tests
	go test ./...

test-race: ## Run all tests with the race detector
	go test -race ./...

cover: ## Run tests with coverage summary
	go test -cover ./...

vet: ## go vet
	go vet ./...

lint: ## golangci-lint (install: https://golangci-lint.run)
	golangci-lint run

fmt: ## Format the code (golangci-lint formatters: gofmt + goimports)
	golangci-lint fmt

tidy: ## Tidy go.mod / go.sum
	go mod tidy

clean: ## Remove build artifacts
	rm -f $(BIN)
