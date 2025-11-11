## Project
BIN      ?= portping
PKG      ?= github.com/sopov/portping
MAIN     ?= ./cmd/portping
OUT      ?= dist

## Versioning
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

## Go flags
GOFLAGS     ?= -trimpath -buildvcs=false -mod=readonly
GO_LDFLAGS  ?= -s -w \
	-X $(PKG)/internal/app.Name=$(BIN) \
	-X $(PKG)/internal/app.Version=$(VERSION) \
	-X $(PKG)/internal/app.Commit=$(COMMIT) \
	-X $(PKG)/internal/app.BuildDate=$(DATE)
BUILD_CMD   ?= go build $(GOFLAGS) -ldflags "$(GO_LDFLAGS)" -o

# docker
DOCKER_IMAGE ?= golang:1.25.4-bookworm
GOOS         ?= $(shell go env GOOS)
GOARCH       ?= $(shell go env GOARCH)
DOCKER_RUN     = docker run --rm -v "$$(pwd)":/src -w /src -e CGO_ENABLED=0 $(DOCKER_IMAGE) bash -c

# crossbuild
define CROSS_BUILD_SCRIPT
set +e; mkdir -p $(OUT); \
PLATFORMS=$$(go tool dist list | grep -E '^(linux|windows|darwin|freebsd|openbsd|netbsd|dragonfly)/'); \
while IFS='/' read -r GOOS GOARCH; do \
  EXT=$$( [ "$$GOOS" = "windows" ] && echo .exe || true ); \
  OUTBIN=$(OUT)/$(BIN)_$${GOOS}_$${GOARCH}$$EXT; \
  echo ">> building $$OUTBIN ($$GOOS/$$GOARCH)"; \
  if GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=0 $(BUILD_CMD) $$OUTBIN $(MAIN); then \
    :; \
  else \
    echo ">> skipped $$OUTBIN (build failed, may require CGO)"; \
  fi; \
done <<< "$$PLATFORMS"
endef
export CROSS_BUILD_SCRIPT

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN{FS=":.*##"; printf "\nTargets:\n"} /^[a-zA-Z0-9_%-]+:.*##/{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: deps
deps: ## Go mod tidy & verify
	@go mod tidy
	@go mod verify

.PHONY: lint
lint: ## golangci-lint
	@golangci-lint run

.PHONY: vet
vet: ## go vet
	@go vet ./...

.PHONY: test
test: ## Run tests with race
	@go test $(GOFLAGS) -race ./...

.PHONY: build
build: ## Build local binary
	@mkdir -p $(OUT)
	@go build $(GOFLAGS) -ldflags '$(GO_LDFLAGS)' -o $(OUT)/$(BIN) $(MAIN)
	@echo "built -> $(OUT)/$(BIN)"

.PHONY: clean
clean: ## Clean dist
	@rm -rf $(OUT)


# -------- Docker builds --------


.PHONY: build-docker
build-docker: ## Build in Docker (single binary)
	@docker run --rm \
	  -v "$$(pwd)":/src -w /src \
	  -e CGO_ENABLED=0 \
	  -e GOOS=$(GOOS) \
	  -e GOARCH=$(GOARCH) \
	  $(DOCKER_IMAGE) \
	  bash -c 'go version && mkdir -p $(OUT) && GOOS=$$GOOS GOARCH=$$GOARCH $(BUILD_CMD) $(OUT)/$(BIN) $(MAIN) && echo "built-in-docker -> $(OUT)/$(BIN) ($$GOOS/$$GOARCH)"'

.PHONY: dist
dist: clean ## Cross-compile to dist/ on host
	@bash -c "$$CROSS_BUILD_SCRIPT"

.PHONY: dist-docker
dist-docker: clean ## Cross-compile matrix in Docker
	@$(DOCKER_RUN) "$$CROSS_BUILD_SCRIPT"