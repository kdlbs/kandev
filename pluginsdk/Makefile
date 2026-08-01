# Standalone authoring SDK and generated kandev.plugin.v1 protocol.

GO := go
TOOLS_BIN := $(CURDIR)/.tools/bin
BUF_VERSION := v1.47.2
PROTOC_GEN_GO_VERSION := v1.36.11
PROTOC_GEN_GO_GRPC_VERSION := v1.5.1

.PHONY: all proto test vet tidy fmt clean

all: test

# Regenerate committed protocol stubs without requiring a system protoc.
proto:
	@echo "Generating pluginsdk proto stubs..."
	@mkdir -p $(TOOLS_BIN)
	GOBIN=$(TOOLS_BIN) GOWORK=off $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(TOOLS_BIN) GOWORK=off $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	cd proto && PATH="$(TOOLS_BIN):$$PATH" GOWORK=off $(GO) run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION) generate

test:
	GOWORK=off $(GO) test -race ./...

vet:
	GOWORK=off $(GO) vet ./...

tidy:
	GOWORK=off $(GO) mod tidy

fmt:
	GOWORK=off $(GO) fmt ./...

clean:
	$(GO) clean
