.PHONY: all build build-all test clean fmt vet

BIN_DIR ?= release

all: build-all

build: controller-darwin-arm64 agent-darwin-arm64

build-all: \
	controller-linux-amd64 agent-linux-amd64 \
	controller-windows-amd64 agent-windows-amd64 \
	controller-darwin-arm64 agent-darwin-arm64

controller-linux-amd64:
	@mkdir -p $(BIN_DIR)/linux-amd64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/linux-amd64/controller ./cmd/controller

agent-linux-amd64:
	@mkdir -p $(BIN_DIR)/linux-amd64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/linux-amd64/agent ./cmd/agent

controller-windows-amd64:
	@mkdir -p $(BIN_DIR)/windows-amd64
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/windows-amd64/controller.exe ./cmd/controller

agent-windows-amd64:
	@mkdir -p $(BIN_DIR)/windows-amd64
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/windows-amd64/agent.exe ./cmd/agent

controller-darwin-arm64:
	@mkdir -p $(BIN_DIR)/darwin-arm64
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/darwin-arm64/controller ./cmd/controller

agent-darwin-arm64:
	@mkdir -p $(BIN_DIR)/darwin-arm64
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/darwin-arm64/agent ./cmd/agent

fmt:
	go fmt ./...

vet:
	go vet ./...

test: vet
	go test ./...

clean:
	rm -rf $(BIN_DIR)
