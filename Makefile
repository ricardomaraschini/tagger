.PHONY: get-code-generator generate build clean

PROJECT=github.com/ricardomaraschini/tagger
GEN_OUTPUT=/tmp/$(PROJECT)/infra/tags

build:
	go build -mod vendor -o _output/bin/tagger ./cmd/tagger
	go build -mod vendor -o _output/bin/kubectl-tag ./cmd/kubectl-tag

get-code-generator:
	rm -rf _output/code-generator
	git clone --depth=1                                                     \
		https://github.com/kubernetes/code-generator.git                \
		_output/code-generator

generate-proto:
	protoc --go-grpc_out=paths=source_relative:.				\
		--go_out=paths=source_relative:.				\
		./infra/pb/*.proto

generate-k8s:
	_output/code-generator/generate-groups.sh all                           \
		$(PROJECT)/infra/tags/v1/gen					\
		$(PROJECT)                                                      \
		infra/tags:v1                                                   \
		--go-header-file _output/code-generator/hack/boilerplate.go.txt \
		--output-base=/tmp
	rm -rf infra/tags/v1/gen
	mv $(GEN_OUTPUT)/v1/* infra/tags/v1/

image:
	podman build -t quay.io/rmarasch/tagger .

clean:
	rm -rf _output
