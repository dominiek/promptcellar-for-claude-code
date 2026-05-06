# E2E test image for the Promptcellar Claude Code plugin.
#
# Drives the *real* `claude` CLI inside a container, installs the plugin via
# one of the two supported routes (curl|sh installer or `/plugin install`
# marketplace flow), runs `claude -p`, and asserts a captured PLF record
# lands in .prompts/.
#
# See test/e2e/README.md for usage; spec at
# promptcellar-specs/product/CLAUDE_CODE_INTEGRATION_TESTS.md.

FROM node:20-bookworm-slim

# Runtime deps:
#   curl, ca-certificates  - download the GH-release tarball, hit get.promptcellar.io
#   git                    - Promptcellar capture is git-aware (off in non-git dirs)
#   jq                     - assertions on captured JSONL records
#   tar                    - tarball extraction (already present, listed for clarity)
#   python3 + jsonschema   - PLF-1 schema validation. Matches the validator used
#                            by the M1/M2/M3 integration suites — keeps the
#                            "what counts as valid" definition single-sourced
#                            and supports draft-2020-12 (ajv-cli only goes to
#                            draft-2019, the PLF schema declares 2020-12).
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      curl ca-certificates git jq tar \
      python3 python3-jsonschema \
 && rm -rf /var/lib/apt/lists/*

# Claude Code itself.
RUN npm install -g @anthropic-ai/claude-code

# Non-root tester user. ${HOME} for plugin cache + git config lives here.
RUN useradd --create-home --shell /bin/bash tester
USER tester
WORKDIR /home/tester

# Identity used as the captured author on the record. Override at test time
# if you want, but a stable default keeps assertions simple.
RUN git config --global user.email "e2e-tester@promptcellar.io" \
 && git config --global user.name  "Promptcellar E2E"

# /repo is bind-mounted from the host (the plugin checkout) at run time so
# scripts can call `bash /repo/install/install.sh` and validate against
# /repo/test/fixtures/plf-1.schema.json without baking either into the image.
# /work is the throwaway git repo each scenario captures into.
# ~/.claude/ is pre-created so the host credentials.json mount lands at
# the expected path with tester ownership (Docker would otherwise create
# the parent dir as root).
RUN mkdir -p /home/tester/work /home/tester/.claude \
 && chmod 700 /home/tester/.claude
WORKDIR /home/tester/work
