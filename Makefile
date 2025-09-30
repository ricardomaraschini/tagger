TAGGER = tagger
PLUGIN = kubectl-image
PLUGIN_DARWIN = kubectl-image-darwin
VERSION ?= v0.0.0
IMAGE_BUILDER ?= docker
IMAGE ?= quay.io/tagger/operator:latest
OUTPUT_DIR ?= output
OUTPUT_BIN = $(OUTPUT_DIR)/bin
OUTPUT_DOC = $(OUTPUT_DIR)/doc
TAGGER_BIN = $(OUTPUT_BIN)/$(TAGGER)
PLUGIN_BIN = $(OUTPUT_BIN)/$(PLUGIN)

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

.PHONY: generate-proto
generate-proto:
	protoc --go-grpc_out=paths=source_relative:. \
		--go_out=paths=source_relative:. \
		./infra/pb/*.proto

.PHONY: generate-crds
generate-crds:
	go tool controller-gen crd paths=./infra/images/v1beta1 output:crd:dir=./chart/templates/

.PHONY: generate-clients
generate-clients:
	./hack/update-codegen.sh

.PHONY: generate
generate: generate-crds generate-clients generate-proto

.PHONY: image
image:
	VERSION=$(VERSION) $(IMAGE_BUILDER) build -f Containerfile -t $(IMAGE) .

.PHONY: clean
clean:
	rm -rf $(OUTPUT_DIR)

.PHONY: pdf
pdf:
	mkdir -p $(OUTPUT_DOC) || true
	pandoc README.md -o $(OUTPUT_DOC)/README.pdf
