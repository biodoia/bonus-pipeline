.PHONY: proto build daemon tui legacy clean all

PROTO_OUT = proto/pipelinepb

all: build

proto:
	@mkdir -p $(PROTO_OUT)
	protoc --go_out=$(PROTO_OUT) --go_opt=paths=source_relative \
	       --go-grpc_out=$(PROTO_OUT) --go-grpc_opt=paths=source_relative \
	       proto/pipeline.proto

build:
	@mkdir -p bin
	go build -o bin/daemon   ./cmd/daemon
	go build -o bin/tui      ./cmd/tui
	go build -o bin/pipeline ./

daemon:
	go run ./cmd/daemon -data .

tui:
	go run ./cmd/tui -data .

legacy:
	go run . $(ARGS)

clean:
	rm -rf bin/ daemon tui pipeline
