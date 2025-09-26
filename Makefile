TAGGER = tagger
PLUGIN = kubectl-image
PLUGIN_DARWIN = kubectl-image-darwin

VERSION ?= v0.0.0
IMAGE_BUILDER ?= podman
IMAGE ?= quay.io/tagger/operator:latest

OUTPUT_DIR ?= output
OUTPUT_BIN = $(OUTPUT_DIR)/bin
OUTPUT_DOC = $(OUTPUT_DIR)/doc

TAGGER_BIN = $(OUTPUT_BIN)/$(TAGGER)
PLUGIN_BIN = $(OUTPUT_BIN)/$(PLUGIN)
GEN_BIN = $(OUTPUT_DIR)/code-generator
KUTTL_BIN = $(OUTPUT_DIR)/kuttl
KUTTL_REPO = https://github.com/kudobuilder/kuttl

PROJECT = github.com/ricardomaraschini/$(TAGGER)
GEN_OUTPUT = /tmp/$(PROJECT)/infra/images

default: build

build: $(TAGGER) $(PLUGIN_DARWIN) $(PLUGIN)

.PHONY: $(TAGGER)
$(TAGGER):
	CGO_ENABLED=0 go build \
		-ldflags="-X 'main.Version=$(VERSION)'" \
		-tags containers_image_openpgp \
		-o $(TAGGER_BIN) \
		./cmd/$(TAGGER)

.PHONY: $(PLUGIN)
$(PLUGIN):
	CGO_ENABLED=0 go build \
		-ldflags="-X 'main.Version=$(VERSION)'" \
		-tags containers_image_openpgp \
		-o $(PLUGIN_BIN) \
		./cmd/$(PLUGIN)

.PHONY: $(PLUGIN_DARWIN)
$(PLUGIN_DARWIN):
	GOOS=darwin GOARCH=amd64 go build \
		-tags containers_image_openpgp \
		-ldflags="-X 'main.Version=$(VERSION)'" \
		-o $(PLUGIN_BIN) \
		./cmd/$(PLUGIN)

.PHONY: get-code-generator
get-code-generator:
	rm -rf $(GEN_BIN) || true
	git clone --depth=1 \
		--branch v0.34.1 \
		https://github.com/kubernetes/code-generator.git \
		$(GEN_BIN)

.PHONY: get-kuttl
get-kuttl:
	rm -rf $(KUTTL_BIN) || true
	mkdir -p $(OUTPUT_DIR) || true
	curl -o $(KUTTL_BIN) -L \
		$(KUTTL_REPO)/releases/download/v0.11.1/kubectl-kuttl_0.11.1_linux_x86_64
	chmod 755 $(KUTTL_BIN)

.PHONY: e2e
e2e:
	$(KUTTL_BIN) test e2e

.PHONY: generate-proto
generate-proto:
	protoc --go-grpc_out=paths=source_relative:. \
		--go_out=paths=source_relative:. \
		./infra/pb/*.proto

.PHONY: generate-k8s
generate-k8s:
	rm -rf $(GEN_OUTPUT) || true
	$(GEN_BIN)/generate-groups.sh all \
		$(PROJECT)/infra/images/v1beta1/gen \
		$(PROJECT) \
		infra/images:v1beta1 \
		--go-header-file=$(GEN_BIN)/hack/boilerplate.go.txt \
		--output-base=/tmp
	rm -rf infra/images/v1beta1/gen
	mv $(GEN_OUTPUT)/v1beta1/* infra/images/v1beta1/

.PHONY: image
image:
	VERSION=$(VERSION) $(IMAGE_BUILDER) build -f Containerfile -t $(IMAGE) .

.PHONY: clean
clean:
	rm -rf $(OUTPUT_DIR)
