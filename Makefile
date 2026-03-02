VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  = -s -w -X github.com/nextlevelbuilder/goclaw/cmd.Version=$(VERSION)
BINARY   = goclaw
UI_DIR   = ui/web
EMBED_DIR = internal/ui/dist

.PHONY: build build-ui build-full run clean version

# Build Go binary only (no UI embed)
build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

# Build the React UI and copy dist into the embed directory
build-ui:
	@echo "→ Installing UI dependencies..."
	cd $(UI_DIR) && pnpm install --frozen-lockfile
	@echo "→ Building UI..."
	cd $(UI_DIR) && pnpm build
	@echo "→ Copying dist to embed directory..."
	rm -rf $(EMBED_DIR)
	cp -r $(UI_DIR)/dist $(EMBED_DIR)
	@echo "✓ UI built and ready to embed"

# Build UI then embed into Go binary (full production build)
build-full: build-ui
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) .
	@echo "✓ $(BINARY) built with embedded UI"

run: build
	./$(BINARY)

run-full: build-full
	./$(BINARY)

clean:
	rm -f $(BINARY)
	rm -rf $(EMBED_DIR)
	mkdir -p $(EMBED_DIR)
	touch $(EMBED_DIR)/.gitkeep

version:
	@echo $(VERSION)

COMPOSE = docker compose -f docker-compose.yml -f docker-compose.managed.yml -f docker-compose.selfservice.yml

up:
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f goclaw
