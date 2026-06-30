.PHONY: all build controller node test clean fmt vet

BIN_DIR ?= release
CONTROLLER_BIN = $(BIN_DIR)/styx-mcp-controller
NODE_BIN = $(BIN_DIR)/styx-mcp-node

all: build

build: controller node

controller:
	@mkdir -p $(BIN_DIR)
	go build -o $(CONTROLLER_BIN) ./cmd/controller

node:
	@mkdir -p $(BIN_DIR)
	go build -o $(NODE_BIN) ./cmd/node

fmt:
	go fmt ./...

vet:
	go vet ./...

test: vet
	go test ./...

clean:
	rm -rf $(BIN_DIR)
