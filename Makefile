BIN         := costblame
INSTALL_DIR := $(HOME)/.local/bin
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
BUILD       := go build -trimpath -ldflags="$(LDFLAGS)"

.PHONY: build install uninstall run dist clean

build: ## build ./costblame for this machine
	$(BUILD) -o $(BIN) .

install: ## build + install to ~/.local/bin (must be on PATH)
	@mkdir -p $(INSTALL_DIR)
	$(BUILD) -o $(INSTALL_DIR)/$(BIN) .
	@echo "installed → $(INSTALL_DIR)/$(BIN)   (run: costblame serve)"

uninstall: ## remove the installed binary
	@rm -f $(INSTALL_DIR)/$(BIN) && echo "removed $(INSTALL_DIR)/$(BIN)"

run: build ## build + start the dashboard
	./$(BIN) serve

dist: ## cross-compile release zips (macOS/Linux/Windows) into dist/
	@mkdir -p dist
	@cp -f RUN.txt dist/ 2>/dev/null || true
	GOOS=darwin  GOARCH=arm64 $(BUILD) -o dist/$(BIN)-macos-arm64 .
	GOOS=darwin  GOARCH=amd64 $(BUILD) -o dist/$(BIN)-macos-intel .
	GOOS=linux   GOARCH=amd64 $(BUILD) -o dist/$(BIN)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 $(BUILD) -o dist/$(BIN)-linux-arm64 .
	GOOS=windows GOARCH=amd64 $(BUILD) -o dist/$(BIN)-windows.exe .
	@cd dist && for p in macos-arm64 macos-intel linux-amd64 linux-arm64; do zip -qj $(BIN)-$$p.zip $(BIN)-$$p RUN.txt; done
	@cd dist && zip -qj $(BIN)-windows.zip $(BIN)-windows.exe RUN.txt
	@echo "built dist/ zips"

clean: ## remove build output
	@rm -f $(BIN) && rm -rf dist && echo "cleaned"
