BINARY_NAME=vior
BUILD_DIR=./tmp
CMD_PATH=./cmd/vior/
PORT?=8080
DISPLAY?=0
FPS?=10
QUALITY?=80

.PHONY: all build run dev clean displays start stop test lint tidy desktop help

## help: Show this help message
help:
	@echo "Vior — Makefile Commands"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
	@echo ""

## build: Build the vior binary
build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@echo "Built: $(BUILD_DIR)/$(BINARY_NAME)"

## run: Build and run vior start
run: build
	$(BUILD_DIR)/$(BINARY_NAME) start -d $(DISPLAY) -p $(PORT) -f $(FPS) -q $(QUALITY)

## dev: Run with Air hot reload
dev:
	air

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)
	go clean

## displays: List connected displays
displays: build
	$(BUILD_DIR)/$(BINARY_NAME) displays

## start: Start streaming (alias for run)
start: run

## stop: Stop streaming (sends interrupt to running instance)
stop:
	@pkill -f "$(BINARY_NAME) start" 2>/dev/null && echo "Stopped." || echo "Not running."

## test: Run all tests
test:
	go test ./... -v

## lint: Run go vet
lint:
	go vet ./...

## tidy: Tidy go modules
tidy:
	go mod tidy

## desktop: Build Wails desktop app
desktop:
	cd desktop && wails build

## desktop-dev: Run Wails desktop app in dev mode
desktop-dev:
	cd desktop && wails dev

## install: Install vior to GOPATH/bin
install:
	go install $(CMD_PATH)
	@echo "Installed: $(shell go env GOPATH)/bin/$(BINARY_NAME)"

## version: Show version
version: build
	$(BUILD_DIR)/$(BINARY_NAME) version
