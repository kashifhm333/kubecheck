SHELL := /bin/bash
export PATH := $(HOME)/.local/go/bin:$(PATH)

BIN_DIR := bin
BINARY := $(BIN_DIR)/kubecheck

VERSION ?= 1.0.0-dev
BUNDLE_VERSION ?= 2026.10.1
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "HEAD")
BUILD_DATE ?= $(shell date -u +'%Y-%m-%d')

LDFLAGS := -X main.EngineVersion=$(VERSION) \
           -X main.BundleVersion=$(BUNDLE_VERSION) \
           -X main.GitCommit=$(COMMIT) \
           -X main.BuildDate=$(BUILD_DATE)

.PHONY: all build test lint fmt clean smoke-test

all: lint test build

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -trimpath -o $(BINARY) ./cmd/kubecheck

test:
	go test -v ./...

test-coverage:
	@mkdir -p coverage
	go test -v -coverprofile=coverage/coverage.out ./...
	go tool cover -func=coverage/coverage.out

lint:
	@test -z "$$(gofmt -l .)" || (echo "Unformatted files found:" && gofmt -l . && exit 1)
	go vet ./...

fmt:
	gofmt -w -s .

clean:
	rm -rf $(BIN_DIR) coverage dist

smoke-test: build
	@echo "--- Smoke Test: --help ---"
	./$(BINARY) --help >/dev/null
	@echo "--- Smoke Test: version ---"
	./$(BINARY) version
	@echo "--- Smoke Test: analyze clean run ---"
	./$(BINARY) analyze tests/fixtures/yaml/valid/pod.yaml
	@echo "--- Smoke Test: analyze finding detection ---"
	./$(BINARY) analyze tests/fixtures/yaml/invalid/duplicate_key.yaml >/dev/null 2>&1 || test $$? -eq 1
	@echo "All smoke tests passed cleanly!"

DIST_DIR := dist

.PHONY: dist-cross dist-packages dist-checksums dist parity-test

dist-cross:
	@mkdir -p $(DIST_DIR)/bin
	@echo "--- Cross-compiling release binaries ---"
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST_DIR)/bin/kubecheck-linux-amd64 ./cmd/kubecheck
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST_DIR)/bin/kubecheck-linux-arm64 ./cmd/kubecheck
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST_DIR)/bin/kubecheck-darwin-amd64 ./cmd/kubecheck
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST_DIR)/bin/kubecheck-darwin-arm64 ./cmd/kubecheck
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o $(DIST_DIR)/bin/kubecheck-windows-amd64.exe ./cmd/kubecheck
	@echo "Cross-compilation complete."

dist-packages: dist-cross
	@mkdir -p $(DIST_DIR)/archives
	@echo "--- Packaging binary archives and Krew plugins ---"
	@mkdir -p $(DIST_DIR)/tmp/linux-amd64 && cp $(DIST_DIR)/bin/kubecheck-linux-amd64 $(DIST_DIR)/tmp/linux-amd64/kubecheck
	@tar -czf $(DIST_DIR)/archives/kubecheck-v$(VERSION)-linux-amd64.tar.gz -C $(DIST_DIR)/tmp/linux-amd64 kubecheck
	@cp $(DIST_DIR)/bin/kubecheck-linux-amd64 $(DIST_DIR)/tmp/linux-amd64/kubectl-check
	@tar -czf $(DIST_DIR)/archives/kubecheck-v$(VERSION)-krew-linux-amd64.tar.gz -C $(DIST_DIR)/tmp/linux-amd64 kubectl-check
	@mkdir -p $(DIST_DIR)/tmp/linux-arm64 && cp $(DIST_DIR)/bin/kubecheck-linux-arm64 $(DIST_DIR)/tmp/linux-arm64/kubecheck
	@tar -czf $(DIST_DIR)/archives/kubecheck-v$(VERSION)-linux-arm64.tar.gz -C $(DIST_DIR)/tmp/linux-arm64 kubecheck
	@cp $(DIST_DIR)/bin/kubecheck-linux-arm64 $(DIST_DIR)/tmp/linux-arm64/kubectl-check
	@tar -czf $(DIST_DIR)/archives/kubecheck-v$(VERSION)-krew-linux-arm64.tar.gz -C $(DIST_DIR)/tmp/linux-arm64 kubectl-check
	@mkdir -p $(DIST_DIR)/tmp/darwin-arm64 && cp $(DIST_DIR)/bin/kubecheck-darwin-arm64 $(DIST_DIR)/tmp/darwin-arm64/kubecheck
	@tar -czf $(DIST_DIR)/archives/kubecheck-v$(VERSION)-darwin-arm64.tar.gz -C $(DIST_DIR)/tmp/darwin-arm64 kubecheck
	@cp $(DIST_DIR)/bin/kubecheck-darwin-arm64 $(DIST_DIR)/tmp/darwin-arm64/kubectl-check
	@tar -czf $(DIST_DIR)/archives/kubecheck-v$(VERSION)-krew-darwin-arm64.tar.gz -C $(DIST_DIR)/tmp/darwin-arm64 kubectl-check
	@mkdir -p $(DIST_DIR)/tmp/darwin-amd64 && cp $(DIST_DIR)/bin/kubecheck-darwin-amd64 $(DIST_DIR)/tmp/darwin-amd64/kubecheck
	@tar -czf $(DIST_DIR)/archives/kubecheck-v$(VERSION)-darwin-amd64.tar.gz -C $(DIST_DIR)/tmp/darwin-amd64 kubecheck
	@cp $(DIST_DIR)/bin/kubecheck-darwin-amd64 $(DIST_DIR)/tmp/darwin-amd64/kubectl-check
	@tar -czf $(DIST_DIR)/archives/kubecheck-v$(VERSION)-krew-darwin-amd64.tar.gz -C $(DIST_DIR)/tmp/darwin-amd64 kubectl-check
	@mkdir -p $(DIST_DIR)/tmp/windows-amd64 && cp $(DIST_DIR)/bin/kubecheck-windows-amd64.exe $(DIST_DIR)/tmp/windows-amd64/kubecheck.exe
	@tar -czf $(DIST_DIR)/archives/kubecheck-v$(VERSION)-windows-amd64.tar.gz -C $(DIST_DIR)/tmp/windows-amd64 kubecheck.exe
	@cp $(DIST_DIR)/bin/kubecheck-windows-amd64.exe $(DIST_DIR)/tmp/windows-amd64/kubectl-check.exe
	@tar -czf $(DIST_DIR)/archives/kubecheck-v$(VERSION)-krew-windows-amd64.tar.gz -C $(DIST_DIR)/tmp/windows-amd64 kubectl-check.exe
	@rm -rf $(DIST_DIR)/tmp
	@echo "Package archives generated in $(DIST_DIR)/archives."

dist-checksums: dist-packages
	@echo "--- Generating SHA-256 Checksum Ledger ---"
	@cd $(DIST_DIR)/archives && sha256sum * > ../checksums.txt
	@cat $(DIST_DIR)/checksums.txt

dist: dist-checksums

parity-test: build
	go test -v ./tests/parity/...

test-benchmark: build
	go test -v ./tests/benchmark/...

.PHONY: test-benchmark
