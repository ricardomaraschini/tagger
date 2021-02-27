APP = tagger
PLUGIN = kubectl-tag

IMAGE_BUILDER ?= podman
IMAGE ?= quay.io/rmarasch/tagger
IMAGE_TAG = $(IMAGE):latest

OUTPUT_DIR ?= _output
OUTPUT_BIN = $(OUTPUT_DIR)/bin

TAGGER_BIN = $(OUTPUT_BIN)/$(APP)
PLUGIN_BIN = $(OUTPUT_BIN)/$(PLUGIN)
GEN_BIN = $(OUTPUT_DIR)/code-generator

PROJECT = github.com/ricardomaraschini/$(APP)
GEN_OUTPUT = /tmp/$(PROJECT)/infra/tags

default: build

build: $(APP) $(PLUGIN)

.PHONY: $(APP)
$(APP):
	go build -o $(TAGGER_BIN) ./cmd/$(APP)

.PHONY: $(PLUGIN)
$(PLUGIN):
	go build -o $(PLUGIN_BIN) ./cmd/$(PLUGIN)

.PHONY: get-code-generator
get-code-generator:
	rm -rf $(GEN_BIN) || true
	git clone --depth=1 \
		https://github.com/kubernetes/code-generator.git \
		$(GEN_BIN)

generate: generate-proto generate-k8s

.PHONY: generate-proto
generate-proto:
	protoc --go-grpc_out=paths=source_relative:. \
		--go_out=paths=source_relative:. \
		./infra/pb/*.proto

.PHONY: generate-k8s
generate-k8s:
	rm -rf $(GEN_OUTPUT) || true
	$(GEN_BIN)/generate-groups.sh all \
		$(PROJECT)/infra/tags/v1/gen \
		$(PROJECT) \
		infra/tags:v1 \
		--go-header-file=$(GEN_BIN)/hack/boilerplate.go.txt \
		--output-base=/tmp
	rm -rf infra/tags/v1/gen
	mv $(GEN_OUTPUT)/v1/* infra/tags/v1/

image:
	$(IMAGE_BUILDER) build --tag=$(IMAGE_TAG) .

.PHONY: clean
clean:
	rm -rf _output
