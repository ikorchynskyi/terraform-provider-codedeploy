HOSTNAME=terraform.local
NAMESPACE=ikorchynskyi
NAME=codedeploy
BINARY=terraform-provider-${NAME}
VERSION=0.1.0
OS_ARCH=linux_amd64

.PHONY: build release install test testacc

default: install

build:
	go build -ldflags='-s -w' -o ${BINARY}

release:
	goreleaser release --rm-dist --snapshot --skip-publish --skip-sign

install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}
	mv ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}

test:
	go test ./... -v $(TESTARGS) -timeout 120m

testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m
