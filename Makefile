GO ?= go
NODE ?= node
NPM ?= npm
APP ?= storyforge
BIN_DIR ?= bin
FRONTEND_DIR ?= web/frontend

.PHONY: build test run release embed frontend-install frontend-build

build: frontend-build
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/$(APP) ./cmd/storyforge

test:
	$(GO) test ./...

run:
	$(GO) run ./cmd/storyforge

release: frontend-build
	scripts/package-release.sh

embed:
	@echo "go:embed resources are compiled during build/run"

frontend-install:
	cd $(FRONTEND_DIR) && $(NPM) install

frontend-build:
	@if [ ! -x "$(FRONTEND_DIR)/node_modules/.bin/vite" ]; then \
		$(MAKE) frontend-install; \
	fi
	cd $(FRONTEND_DIR) && $(NODE) node_modules/vite/bin/vite.js build
