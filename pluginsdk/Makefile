GO ?= go
PROTO_TOOLS_BIN := $(CURDIR)/.tools/bin
BUF_VERSION := v1.47.2
PROTOC_GEN_GO_VERSION := v1.36.11
PROTOC_GEN_GO_GRPC_VERSION := v1.5.1

.PHONY: proto test vet
proto:
	mkdir -p $(PROTO_TOOLS_BIN)
	GOBIN=$(PROTO_TOOLS_BIN) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(PROTO_TOOLS_BIN) $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	cd proto && PATH="$(PROTO_TOOLS_BIN):$$PATH" $(GO) run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION) generate

test:
	$(GO) test -trimpath -race ./...

vet:
	$(GO) vet ./...
