.PHONY: build setup frontend-setup go-setup frontend-build frontend-test go-test vet test check clean dev

BINARY ?= pi-web
WEB_DIR := web
NODE_MODULES := $(WEB_DIR)/node_modules

build: setup frontend-build
	go build -o $(BINARY) .

setup: frontend-setup go-setup

frontend-setup:
	@if [ ! -d "$(NODE_MODULES)" ]; then \
		echo "Installing frontend dependencies..."; \
		cd $(WEB_DIR) && npm install; \
	else \
		echo "Frontend dependencies already installed."; \
	fi

go-setup:
	go mod download

frontend-build: frontend-setup
	cd $(WEB_DIR) && npm run build

frontend-test: frontend-setup
	cd $(WEB_DIR) && npm run test

go-test: go-setup
	go test ./...

vet: go-setup
	go vet ./...

test: frontend-test go-test

check: frontend-test frontend-build go-test vet

dev: frontend-setup go-setup
	@echo "Starting dev mode (frontend watcher + Go hot-reloader)..."
	@cd $(WEB_DIR) && npm run dev & \
	VITE_PID=$$!; \
	trap "kill $$VITE_PID 2>/dev/null; exit" INT TERM EXIT; \
	air

clean:
	rm -f $(BINARY)
	rm -rf $(WEB_DIR)/dist
