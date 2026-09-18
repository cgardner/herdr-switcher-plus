BINARY      := herdr-switcher-plus
VERSION     := $(shell sed -n 's/^version = "\(.*\)"/\1/p' herdr-plugin.toml | head -1)
LDFLAGS     := -s -w -X github.com/cgardner/herdr-switcher-plus/internal/cli.version=$(VERSION)
COVER_MIN   := 99.0
DIST        := dist

# Platforms released as prebuilt binaries. scripts/install.sh maps uname
# output onto these same names, so the two lists must stay in step.
PLATFORMS := darwin-arm64 darwin-amd64 linux-amd64 linux-arm64

.DEFAULT_GOAL := build

.PHONY: build
build: ## Build the plugin binary into bin/
	@mkdir -p bin
	@go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) .
	@echo "built bin/$(BINARY) $(VERSION)"

.PHONY: test
test: ## Run the test suite
	@go test ./...

.PHONY: cover
cover: ## Run tests and fail below COVER_MIN percent of statements
	@go test ./... -covermode=count -coverprofile=cover.out >/dev/null
	@go tool cover -func=cover.out | tail -1
	@go tool cover -func=cover.out | tail -1 | awk '{gsub(/%/,"",$$3); \
	  if ($$3+0 < $(COVER_MIN)) { printf "coverage %.1f%% is below the %s%% floor\n", $$3, "$(COVER_MIN)"; exit 1 } }'

.PHONY: cover-html
cover-html: cover ## Open the coverage report in a browser
	@go tool cover -html=cover.out

.PHONY: fmt
fmt: ## Rewrite sources with gofmt
	@gofmt -w .

.PHONY: lint
lint: ## Fail if anything is unformatted or vet reports a problem
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	@go vet ./...

.PHONY: ci
ci: lint cover ## Everything the CI workflow runs

.PHONY: dist
dist: ## Cross-compile a release binary for every platform
	@rm -rf $(DIST) && mkdir -p $(DIST)
	@for p in $(PLATFORMS); do \
	  os=$${p%-*}; arch=$${p#*-}; \
	  GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
	    go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-$$p . || exit 1; \
	  echo "built $(DIST)/$(BINARY)-$$p"; \
	done

.PHONY: checksums
checksums: dist ## Write a SHA256SUMS file beside the release binaries
	@cd $(DIST) && shasum -a 256 $(BINARY)-* > SHA256SUMS && cat SHA256SUMS

.PHONY: link
link: build ## Link this working copy into the running Herdr session
	@herdr plugin link "$(CURDIR)" >/dev/null
	@herdr plugin list --plugin cgardner.$(BINARY) --json >/dev/null && echo "linked $(CURDIR)"

.PHONY: unlink
unlink: ## Remove the linked plugin from the running Herdr session
	@herdr plugin unlink cgardner.$(BINARY) >/dev/null && echo "unlinked"

.PHONY: run
run: build ## Print the agent list without opening a pane
	@./bin/$(BINARY) --list

.PHONY: version
version: ## Print the version recorded in herdr-plugin.toml
	@echo $(VERSION)

.PHONY: clean
clean: ## Remove build output
	@rm -rf bin $(DIST) cover.out

.PHONY: help
help: ## List the targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  %-12s %s\n", $$1, $$2}'
