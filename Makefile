# ########################################################## #
# Makefile for Golang Project
# Includes cross-compiling, installation, cleanup
# ########################################################## #

ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

BINARY=pcp
VERSION=0.1.0
BUILD=`git rev-parse HEAD`
PLATFORMS=darwin linux windows
ARCHITECTURES=386 amd64 arm

# Setup linker flags option for build that interoperate with variable names in src code
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.Build=${BUILD}"

default: build

all: clean release install

build:
	go build ${LDFLAGS} -o ${BINARY} cmd/pcp/pcp.go

release:
	$(foreach GOOS, $(PLATFORMS),\
	$(foreach GOARCH, $(ARCHITECTURES), $(shell export GOOS=$(GOOS); export GOARCH=$(GOARCH); go build -o out/$(BINARY)-$(GOOS)-$(GOARCH) cmd/pcp/pcp.go)))

install:
	go install ${LDFLAGS}

format:
	gofumpt -l -w .

tools:
	go get mvdan.cc/gofumpt

# Remove only what we've created
clean:
	rm -r out

.PHONY: check clean install release all
