.PHONY: all build clean test test-all install dev-install cross-build

GOFLAGS := -trimpath -ldflags=-s\ -w
BIN := plugin/bin
COMMANDS := pc-hook-session pc-hook-prompt pc-hook-tool pc-hook-stop pc-cli pc-mcp

all: build

build:
	@mkdir -p $(BIN)
	@for cmd in $(COMMANDS); do \
		go build $(GOFLAGS) -o $(BIN)/$$cmd ./cmd/$$cmd ; \
	done
	@echo "built: $(BIN)/{$(shell echo $(COMMANDS) | tr ' ' ',')}"

test:
	go test ./...

test-all: build test
	bash test/m1_integration.sh
	bash test/m3_integration.sh
	bash test/m2_integration.sh

dev-install: build
	bash install/dev-install.sh

# Cross-compile every binary for every supported platform into dist/<platform>/.
# Used by CI to produce release artefacts.
PLATFORMS := darwin-arm64 darwin-x64 linux-arm64 linux-x64 windows-x64
cross-build:
	@for platform in $(PLATFORMS); do \
		case $$platform in \
			darwin-arm64) GOOS=darwin GOARCH=arm64 ;; \
			darwin-x64)   GOOS=darwin GOARCH=amd64 ;; \
			linux-arm64)  GOOS=linux  GOARCH=arm64 ;; \
			linux-x64)    GOOS=linux  GOARCH=amd64 ;; \
			windows-x64)  GOOS=windows GOARCH=amd64 ;; \
		esac ; \
		out=dist/$$platform/plugin ; \
		mkdir -p $$out/bin ; \
		cp -R plugin/.claude-plugin plugin/hooks plugin/commands plugin/.mcp.json $$out/ 2>/dev/null || true ; \
		for cmd in $(COMMANDS); do \
			ext= ; case $$platform in windows-x64) ext=.exe ;; esac ; \
			GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=0 go build $(GOFLAGS) -o $$out/bin/$${cmd}$${ext} ./cmd/$$cmd ; \
		done ; \
		echo "$$platform → $$out" ; \
	done

clean:
	rm -rf $(BIN) dist/
