.PHONY: get-code-generator generate build clean

PROJECT=github.com/ricardomaraschini/it
GEN_OUTPUT=/tmp/$(PROJECT)/imagetags

get-code-generator:
	rm -rf _output
	git clone --depth=1                                                     \
		https://github.com/kubernetes/code-generator.git                \
		_output/code-generator

generate:
	_output/code-generator/generate-groups.sh all                           \
		$(PROJECT)/imagetags/generated                                  \
		$(PROJECT)                                                      \
		imagetags:v1                                                    \
		--go-header-file _output/code-generator/hack/boilerplate.go.txt \
		--output-base=/tmp
	rm -rf imagetags/generated
	mv $(GEN_OUTPUT)/generated imagetags 
	mv $(GEN_OUTPUT)/v1/* imagetags/v1/

build:
	go build -o it ./cmd/

clean:
	rm -rf _output
	rm -rf it