BINARY=pcp
VERSION=0.3.0
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
	$(foreach GOARCH, $(ARCHITECTURES), $(shell export GOOS=$(GOOS); export GOARCH=$(GOARCH); go build ${LDFLAGS} -o out/$(BINARY)-$(GOOS)-$(GOARCH) cmd/pcp/pcp.go)))

install:
	go install ${LDFLAGS}

format:
	gofumpt -w -l .

proto:
	protoc -I=pkg/pb --go_out=pkg/pb --go_opt=paths=source_relative p2p.proto
	gofumpt -w -l ./pkg/pb/

tools:
	go install mvdan.cc/gofumpt
	go install google.golang.org/protobuf/cmd/protoc-gen-go

# Remove only what we've created
clean:
	rm -r out

.PHONY: check clean install release all
