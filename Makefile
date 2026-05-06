.PHONY: all build clean test test-all test-e2e install dev-install cross-build

GOFLAGS := -trimpath -ldflags=-s\ -w
# Real binaries land in .real/ alongside the committed shell shims at plugin/bin/.
# The shims exec ./.real/<name> at runtime, falling back to a SessionStart
# bootstrap that downloads the platform tarball when .real/ is empty.
BIN := plugin/bin/.real
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
		mkdir -p $$out/bin/.real ; \
		cp -R plugin/.claude-plugin plugin/hooks plugin/commands plugin/.mcp.json $$out/ 2>/dev/null || true ; \
		for shim in plugin/bin/pc-hook-session plugin/bin/pc-hook-prompt plugin/bin/pc-hook-tool plugin/bin/pc-hook-stop plugin/bin/pc-cli plugin/bin/pc-mcp plugin/bin/_bootstrap.sh ; do \
			cp $$shim $$out/bin/ ; \
		done ; \
		for cmd in $(COMMANDS); do \
			ext= ; case $$platform in windows-x64) ext=.exe ;; esac ; \
			GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=0 go build $(GOFLAGS) -o $$out/bin/.real/$${cmd}$${ext} ./cmd/$$cmd ; \
		done ; \
		echo "$$platform → $$out" ; \
	done

# End-to-end tests: drive the real `claude` CLI inside a Docker container,
# install the plugin via each of the two supported routes, fire `claude -p`,
# assert a captured PLF record landed. See test/e2e/README.md for details.
#
# Auth is resolved by test/e2e/run.sh in priority order:
#   1. $ANTHROPIC_API_KEY env var
#   2. macOS keychain "Claude Code-credentials"
#   3. ~/.claude/.credentials.json
# Whatever the host already uses for `claude` works — no separate key needed.
test-e2e:
	bash test/e2e/run.sh

clean:
	rm -rf $(BIN) dist/
