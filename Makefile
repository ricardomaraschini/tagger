.PHONY: get-code-generator generate build clean

PROJECT=github.com/ricardomaraschini/tagger
GEN_OUTPUT=/tmp/$(PROJECT)/imagetags

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
		./imagetags/pb/*.proto

generate-k8s:
	_output/code-generator/generate-groups.sh all                           \
		$(PROJECT)/imagetags/generated                                  \
		$(PROJECT)                                                      \
		imagetags:v1                                                    \
		--go-header-file _output/code-generator/hack/boilerplate.go.txt \
		--output-base=/tmp
	rm -rf imagetags/generated
	mv $(GEN_OUTPUT)/generated imagetags 
	mv $(GEN_OUTPUT)/v1/* imagetags/v1/

image:
	podman build -t quay.io/rmarasch/tagger .

clean:
	rm -rf _output
