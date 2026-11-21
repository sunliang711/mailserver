BINARY := mailserver
CMD := .
WEB_DIR := web
CGO_ENABLED ?= 0
VERSION ?= $(or $(shell git describe --tags --always --dirty 2>/dev/null),dev)
LDFLAGS ?= -s -w -X main.version=$(VERSION)

export PATH := $(PATH):/usr/local/go/bin:/opt/homebrew/bin:$(HOME)/.nvm/versions/node/v24.14.1/bin
GO ?= $(or $(shell PATH="$(PATH)" command -v go),go)
NPM ?= $(or $(shell PATH="$(PATH)" command -v npm),npm)

.DEFAULT_GOAL := build

.PHONY: all build web web-deps test vet run dev-web docker clean help

all: build

web-deps:
	@if [ ! -d "$(WEB_DIR)/node_modules" ]; then \
		cd "$(WEB_DIR)" && "$(NPM)" ci; \
	fi

web: web-deps
	cd "$(WEB_DIR)" && "$(NPM)" run build

build: web
	CGO_ENABLED=$(CGO_ENABLED) "$(GO)" build -trimpath -ldflags="$(LDFLAGS)" -o "$(BINARY).tmp" $(CMD)
	mv "$(BINARY).tmp" "$(BINARY)"
	@echo "Built ./$(BINARY)"

vet: web
	"$(GO)" vet ./...

test: web
	"$(GO)" test -count=1 -race ./...

run: build
	./$(BINARY)

dev-web: web-deps
	cd "$(WEB_DIR)" && "$(NPM)" run dev

docker:
	docker build -t mailserver:latest .

clean:
	rm -f "$(BINARY)" "$(BINARY).tmp"
	rm -rf "$(WEB_DIR)/dist"

help:
	@echo "Available targets:"
	@echo "  make build    - Build web assets and ./$(BINARY) executable (default)"
	@echo "  make web      - Build React + Vite assets into $(WEB_DIR)/dist"
	@echo "  make test     - Build web assets and run Go tests with -race"
	@echo "  make vet      - Run go vet ./..."
	@echo "  make run      - Build and start ./$(BINARY)"
	@echo "  make dev-web  - Start Vite dev server"
	@echo "  make docker   - Build Docker image mailserver:latest"
	@echo "  make clean    - Remove built binary and $(WEB_DIR)/dist"
