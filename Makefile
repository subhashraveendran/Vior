BINARY_NAME=vior
BUILD_DIR=./tmp
DIST_DIR=./dist
CMD_PATH=./cmd/vior/
PORT?=8080
DISPLAY?=0
FPS?=10
QUALITY?=80

# Release version: defaults to the nearest git tag (falls back for untagged trees).
VERSION?=$(shell git describe --tags --always 2>/dev/null || echo v0.0.0-dev)
GOOS_HOST=$(shell go env GOOS)
GOARCH_HOST=$(shell go env GOARCH)
ifeq ($(GOOS_HOST),windows)
RELEASE_EXT=.exe
else
RELEASE_EXT=
endif
RELEASE_BINARY=$(BINARY_NAME)-$(VERSION)-$(GOOS_HOST)-$(GOARCH_HOST)$(RELEASE_EXT)

.PHONY: all build run dev clean displays start stop test lint staticcheck tidy desktop release help

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
	rm -rf $(BUILD_DIR) $(DIST_DIR)
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

## staticcheck: Run staticcheck (installs the pinned version if missing)
staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck@2026.1
	"$$(go env GOPATH)/bin/staticcheck" ./cmd/... ./internal/...

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

## macos-install: Build Wails app, copy to /Applications, strip quarantine
macos-install: desktop
	@if [ ! -d "desktop/build/bin" ]; then echo "No build found — run 'make desktop' first"; exit 1; fi
	@APP=$$(ls -t desktop/build/bin/*.app 2>/dev/null | head -1); \
	if [ -z "$$APP" ]; then echo "No .app found in desktop/build/bin/"; exit 1; fi; \
	rm -rf /Applications/vior-app.app; \
	cp -R "$$APP" /Applications/vior-app.app; \
	xattr -dr com.apple.quarantine /Applications/vior-app.app; \
	echo "Installed: /Applications/vior-app.app (quarantine cleared)"

## version: Show version
version: build
	$(BUILD_DIR)/$(BINARY_NAME) version

## release: Build a versioned CLI binary into dist/ with SHA-256 checksums
##          Usage: make release [VERSION=v1.2.3] [PUBLISH=1]
##          VERSION defaults to `git describe --tags`. The CLI depends on CGO
##          (libusb, X11), so cross-compilation is not possible — run this
##          target once on each platform you want to ship.
##          PUBLISH=1 additionally uploads the artifacts to a GitHub release
##          named $(VERSION) via the `gh` CLI, if it is installed.
release:
	go vet ./cmd/... ./internal/...
	go test ./cmd/... ./internal/... -short
	@mkdir -p $(DIST_DIR)
	go build -trimpath -ldflags "-s -w" -o $(DIST_DIR)/$(RELEASE_BINARY) $(CMD_PATH)
	@cd $(DIST_DIR) && { command -v sha256sum >/dev/null 2>&1 && sha256sum $(BINARY_NAME)-* || shasum -a 256 $(BINARY_NAME)-*; } > checksums.txt
	@echo "Release artifacts in $(DIST_DIR):"
	@ls -l $(DIST_DIR)
	@if [ "$(PUBLISH)" = "1" ]; then \
		if command -v gh >/dev/null 2>&1; then \
			gh release create "$(VERSION)" $(DIST_DIR)/$(BINARY_NAME)-* $(DIST_DIR)/checksums.txt --title "$(VERSION)" --generate-notes; \
		else \
			echo "PUBLISH=1 set but the GitHub CLI (gh) is not installed — upload $(DIST_DIR) manually"; \
			exit 1; \
		fi \
	fi
