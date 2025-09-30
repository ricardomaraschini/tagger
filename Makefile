VERSION ?= v0.0.0
IMAGE ?= quay.io/tagger/operator:latest

default: build

.PHONY: build
build: tagger kubectl-image kubectl-image-darwin

.PHONY: tagger
tagger:
	CGO_ENABLED=0 go build                          \
		-ldflags="-X 'main.Version=$(VERSION)'" \
		-tags containers_image_openpgp          \
		-o output/bin/tagger                    \
		./cmd/tagger

.PHONY: kubectl-image
kubectl-image:
	CGO_ENABLED=0 go build \
		-ldflags="-X 'main.Version=$(VERSION)'" \
		-tags containers_image_openpgp          \
		-o output/bin/kubectl-image             \
		./cmd/kubectl-image

.PHONY: kubectl-image-darwin
kubectl-image-darwin:
	GOOS=darwin GOARCH=amd64 go build               \
		-tags containers_image_openpgp          \
		-ldflags="-X 'main.Version=$(VERSION)'" \
		-o output/bin/kubectl-image             \
		./cmd/kubectl-image

.PHONY: generate-proto
generate-proto:
	protoc --go-grpc_out=paths=source_relative:. \
		--go_out=paths=source_relative:.     \
		./infra/pb/*.proto

.PHONY: generate-crds
generate-crds:
	go tool controller-gen crd                \
		output:crd:dir=./chart/templates/ \
		paths=./infra/images/v1beta1

.PHONY: generate-clients
generate-clients:
	./hack/update-codegen.sh

.PHONY: generate
generate: generate-crds generate-clients generate-proto

.PHONY: image
image:
	VERSION=$(VERSION) docker build -f Containerfile -t $(IMAGE) .

.PHONY: clean
clean:
	rm -rf output

.PHONY: pdf
pdf:
	mkdir -p output/doc || true
	pandoc README.md -o output/doc/README.pdf

# creates a test cluster locally using kind. this installs the cluster using a
# custom network cidr that may not work for everyone. we use a custom cidr to
# be able to properly configure metal lb with a working address pool. feel free
# to edit the ranges if needed but remember to adjust the metal lb config.
.PHONY: create-test-cluster
create-test-cluster:
	docker network create            \
		--scope local            \
		--gateway 172.18.100.1   \
		--driver bridge          \
		--subnet 172.18.100.0/24 \
		kind
	kind create cluster                                      \
		--config .github/workflows/etc/kind-cluster.yaml \
		--name kind
	kubectl create namespace metallb
	helm repo add metallb https://metallb.github.io/metallb
	helm install --wait -n metallb metallb metallb/metallb
	kubectl apply -f ./.github/workflows/etc/metallb.yaml
