default: build

all: clean install

test:
	go test ./...

build:
	go build -o dist/pcp cmd/pcp/pcp.go

install:
	go install cmd/pcp/pcp.go

format:
	gofumpt -w -l .

proto:
	protoc -I=pkg/pb --go_out=pkg/pb --go_opt=paths=source_relative p2p.proto
	gofumpt -w -l ./pkg/pb/

tools:
	go install mvdan.cc/gofumpt@v0.1.1
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26.0
	go install github.com/golang/mock/mockgen@v1.5.0

gen:
	go generate ./...

# Remove only what we've created
clean:
	rm -r dist

.PHONY: all clean test install release proto format tools gen
