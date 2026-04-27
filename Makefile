BINARY_NAME ?= quickpress
BUILD_DIR ?= dist
LINUX_AMD64_BINARY := $(BUILD_DIR)/$(BINARY_NAME)

.PHONY: help test build-linux-amd64 clean

help:
	@echo "Available targets:"
	@echo "  make test                 Run all Go tests"
	@echo "  make build-linux-amd64    Build Linux x86_64 binary"
	@echo "  make clean                Remove build outputs"

test:
	go test ./...

build-linux-amd64:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(LINUX_AMD64_BINARY) .
	@echo "Built $(LINUX_AMD64_BINARY)"

clean:
	rm -rf $(BUILD_DIR)