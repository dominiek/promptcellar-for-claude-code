.PHONY: all build clean test

GOFLAGS := -trimpath -ldflags=-s\ -w
BIN := plugin/bin

all: build

build:
	@mkdir -p $(BIN)
	go build $(GOFLAGS) -o $(BIN)/pc-hook-session ./cmd/pc-hook-session
	go build $(GOFLAGS) -o $(BIN)/pc-hook-prompt  ./cmd/pc-hook-prompt
	go build $(GOFLAGS) -o $(BIN)/pc-hook-tool    ./cmd/pc-hook-tool
	go build $(GOFLAGS) -o $(BIN)/pc-hook-stop    ./cmd/pc-hook-stop
	@echo "built: $(BIN)/{pc-hook-session,pc-hook-prompt,pc-hook-tool,pc-hook-stop}"

test:
	go test ./...

clean:
	rm -rf $(BIN)
