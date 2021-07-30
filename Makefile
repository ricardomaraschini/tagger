APP = tagger
PLUGIN = kubectl-tag
PLUGIN_DARWIN = kubectl-tag-darwin

VERSION ?= v0.0.0
BUNDLE_VERSION ?= 0.0.0
IMAGE_BUILDER ?= podman
BUNDLE_IMAGE ?= quay.io/tagger/olm-bundle
IMAGE ?= quay.io/tagger/operator
IMAGE_TAG = $(IMAGE):latest
BUNDLE_TAG = $(IMAGE):latest

OUTPUT_DIR ?= output
OUTPUT_BIN = $(OUTPUT_DIR)/bin
OUTPUT_DOC = $(OUTPUT_DIR)/doc
OUTPUT_OLM_BUNDLE = $(OUTPUT_DIR)/bundle

TAGGER_BIN = $(OUTPUT_BIN)/$(APP)
PLUGIN_BIN = $(OUTPUT_BIN)/$(PLUGIN)
GEN_BIN = $(OUTPUT_DIR)/code-generator

PROJECT = github.com/ricardomaraschini/$(APP)
GEN_OUTPUT = /tmp/$(PROJECT)/infra/tags

default: build

build: $(APP) $(PLUGIN)

.PHONY: $(APP)
$(APP):
	go build \
		-ldflags="-X 'main.Version=$(VERSION)'" \
		-o $(TAGGER_BIN) \
		./cmd/$(APP)

.PHONY: $(PLUGIN)
$(PLUGIN):
	go build \
		-ldflags="-X 'main.Version=$(VERSION)'" \
		-o $(PLUGIN_BIN) \
		./cmd/$(PLUGIN)

.PHONY: $(PLUGIN_DARWIN)
$(PLUGIN_DARWIN):
	GOOS=darwin GOARCH=amd64 \
		go build -tags containers_image_openpgp \
		-ldflags="-X 'main.Version=$(VERSION)'" \
		-o $(PLUGIN_BIN) \
		./cmd/$(PLUGIN)

.PHONY: get-code-generator
get-code-generator:
	rm -rf $(GEN_BIN) || true
	git clone --depth=1 \
		--branch v0.21.1 \
		https://github.com/kubernetes/code-generator.git \
		$(GEN_BIN)

.PHONY: generate-proto
generate-proto:
	protoc --go-grpc_out=paths=source_relative:. \
		--go_out=paths=source_relative:. \
		./infra/pb/*.proto


.PHONY: generate-bundle
generate-bundle:
	operator-sdk generate bundle \
		--input-dir olm/  \
		--package tagger \
		--version $(BUNDLE_VERSION) \
		--output-dir $(OUTPUT_OLM_BUNDLE)

.PHONY: bundle
bundle: generate-bundle
	$(IMAGE_BUILDER) build -f bundle.Dockerfile --tag=$(BUNDLE_TAG) .

.PHONY: generate-k8s
generate-k8s:
	rm -rf $(GEN_OUTPUT) || true
	$(GEN_BIN)/generate-groups.sh all \
		$(PROJECT)/infra/tags/v1beta1/gen \
		$(PROJECT) \
		infra/tags:v1beta1 \
		--go-header-file=$(GEN_BIN)/hack/boilerplate.go.txt \
		--output-base=/tmp
	rm -rf infra/tags/v1beta1/gen
	mv $(GEN_OUTPUT)/v1beta1/* infra/tags/v1beta1/

image:
	$(IMAGE_BUILDER) build -f Containerfile --tag=$(IMAGE_TAG) .

.PHONY: clean
clean:
	rm -rf $(OUTPUT_DIR)
	rm -rf bundle.Dockerfile

.PHONY: pdf
pdf:
	mkdir -p $(OUTPUT_DOC)
	grep -v badge.svg README.md | pandoc \
		-fmarkdown-implicit_figures \
		-V geometry:margin=1in \
		-o $(OUTPUT_DOC)/README.pdf
