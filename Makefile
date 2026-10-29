SHELL := /bin/bash

GO ?= go
GOFMT ?= gofmt
BIN_NAME ?= webctx
CMD_PATH ?= ./cmd/$(BIN_NAME)
DIST_DIR ?= dist
BIN_PATH ?= $(DIST_DIR)/$(BIN_NAME)
VERSION ?= $(shell node -p "require('./npm/package.json').version" 2>/dev/null)
LDFLAGS ?= -s -w -X github.com/amxv/webctx/internal/buildinfo.Version=$(if $(VERSION),$(VERSION),dev)

.PHONY: help fmt test vet lint check build build-all install-local clean release-tag docs-check docs-build

help:
	@echo "webctx command runner"
	@echo ""
	@echo "Targets:"
	@echo "  make fmt          - format Go files"
	@echo "  make test         - run go test ./..."
	@echo "  make vet          - run go vet ./..."
	@echo "  make lint         - run npm CLI packaging and deployment contract tests"
	@echo "  make check        - fmt + test + vet + lint"
	@echo "  make docs-check   - check Astro docs from docs/"
	@echo "  make docs-build   - build the Astro docs site"
	@echo "  make build        - build local binary to dist/webctx"
	@echo "  make build-all    - build release binaries for 7 target platforms"
	@echo "  make install-local - install CLI to ~/.local/bin/webctx"
	@echo "  make clean        - remove generated build and package artifacts"
	@echo "  make release-tag  - create and push git tag (requires VERSION=x.y.z)"

fmt:
	@$(GOFMT) -w $$(find api cmd internal pkg -type f -name '*.go')

test:
	@$(GO) test ./...

vet:
	@$(GO) vet ./...

lint:
	@npm --prefix npm run lint
	@node --test tests/origo-deploy.test.mjs

docs-check:
	@cd docs && bun run check

docs-build:
	@cd docs && bun run build

check: fmt test vet lint

build:
	@mkdir -p $(DIST_DIR)
	@$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN_PATH) $(CMD_PATH)

build-all:
	@mkdir -p $(DIST_DIR)
	@for target in "darwin amd64" "darwin arm64" "linux amd64" "linux arm64" "windows 386" "windows amd64" "windows arm64"; do \
		set -- $$target; \
		GOOS=$$1; GOARCH=$$2; \
		EXT=""; \
		if [ "$$GOOS" = "windows" ]; then EXT=".exe"; fi; \
		echo "Building $(BIN_NAME) for $$GOOS/$$GOARCH"; \
		CGO_ENABLED=0 GOOS=$$GOOS GOARCH=$$GOARCH $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o "$(DIST_DIR)/$(BIN_NAME)_$${GOOS}_$${GOARCH}$${EXT}" $(CMD_PATH); \
	done

install-local: build
	@mkdir -p $$HOME/.local/bin
	@install -m 755 $(BIN_PATH) $$HOME/.local/bin/$(BIN_NAME)
	@echo "Installed $(BIN_NAME) to $$HOME/.local/bin/$(BIN_NAME)"

clean:
	@rm -rf $(DIST_DIR) npm/.publish docs/dist docs/.astro .vercel/output

release-tag:
	@test -n "$(VERSION)" || (echo "Usage: make release-tag VERSION=x.y.z" && exit 1)
	@git tag "v$(VERSION)"
	@git push origin "v$(VERSION)"
