# Promptcellar — Claude Code end-to-end tests

Black-box tests that drive the real `claude` CLI inside Docker, install the
plugin via each of the two supported routes, and assert a captured PLF
record actually lands in `.prompts/`.

Spec: [`promptcellar-specs/product/CLAUDE_CODE_INTEGRATION_TESTS.md`](https://github.com/dominiek/promptcellar-specs/blob/main/product/CLAUDE_CODE_INTEGRATION_TESTS.md).

## Why these exist

The unit + M1–M5 integration suites under `test/` exercise the hook binaries
and MCP server *directly* — synthetic stdin in, validated record out. That
gives high confidence in the capture layer but cannot catch regressions in
the install layer.

PR #14 fixed exactly such a regression: `claude plugin install` reported
success but every hook silently failed because `plugin/bin/` was gitignored.
Nothing in the existing tests noticed. These E2E tests close that gap.

## Scenarios

| ID    | Driver                          | Tests                                                                                  |
| ----- | ------------------------------- | -------------------------------------------------------------------------------------- |
| E2E-A | `run-curl-install.sh`           | `bash install/install.sh` route — equivalent to `curl -fsSL https://get.promptcellar.io/claude-code \| sh`. |
| E2E-B | `run-marketplace-install.sh`    | `claude plugin install promptcellar@promptcellar` route ONLY (no Phase 2 binary fetch). Exercises the SessionStart bootstrap shim. |

Both fire `claude -p "what time is it"` after install, then run the shared
[`assert-prompt-captured.sh`](./assert-prompt-captured.sh) helper against
`.prompts/`.

## Running locally

```sh
# Use a dedicated CI-only API key, NOT your personal Anthropic account.
export ANTHROPIC_API_KEY=sk-ant-...

make test-e2e
```

That's it. The Makefile target builds the image (`promptcellar-e2e:latest`)
and runs both scenarios in sequence. Total wall clock ~60–90s on the first
run, ~30–45s thereafter (image cached).

To run a single scenario:

```sh
docker build -t promptcellar-e2e:latest -f test/e2e/Dockerfile test/e2e
docker run --rm -e ANTHROPIC_API_KEY -v "$PWD:/repo:ro" \
  promptcellar-e2e:latest bash /repo/test/e2e/run-marketplace-install.sh
```

## Auth — what NOT to do

**Do not mount `~/.claude/` from your host into the container.** It contains
your OAuth session, login state, and possibly other plugin state. Mounting
it would leak your personal identity into container layers and any saved
test artifacts. The container uses the `ANTHROPIC_API_KEY` env var only —
that's the hermetic, revocable path.

If you don't have a key handy, create one at
<https://console.anthropic.com/settings/keys> and tag it for E2E use only.

## What each scenario asserts

`assert-prompt-captured.sh` runs after `claude -p` completes:

1. Exactly one `.jsonl` file under `<work>/.prompts/YYYY/MM/DD/`.
2. Exactly one JSON record in that file.
3. Record validates against `test/fixtures/plf-1.schema.json`.
4. `record.prompt` matches the prompt we sent.
5. `record.outcome.status == "completed"`.
6. `record.author.email` is non-empty (proves git identity flowed through).

E2E-B *additionally* asserts that
`~/.claude/plugins/cache/promptcellar/promptcellar/<ver>/bin/.real/pc-hook-session`
exists *after* `claude -p` — proving the SessionStart bootstrap actually
fetched the platform binary from the GH release. Without that check we'd be
testing capture (already covered by other suites) but not the install bug.

## Environment overrides

| Var                | Default                                  | Notes                                                            |
| ------------------ | ---------------------------------------- | ---------------------------------------------------------------- |
| `ANTHROPIC_API_KEY`| **required**                             | Passed into the container via `-e`.                              |
| `EXPECTED_PROMPT`  | `what time is it`                        | Sent to `claude -p` and asserted on the captured record.         |
| `PC_INSTALLER`     | `bash /repo/install/install.sh`          | E2E-A only. Set to `curl -fsSL https://get.promptcellar.io/claude-code \| sh` to test the deployed installer URL too. |
| `PC_MARKETPLACE`   | `dominiek/promptcellar-for-claude-code`  | E2E-B only. The GH repo registered as the marketplace.           |

## Cost & runtime

Each scenario fires one `claude -p` call. With Claude Code's default model,
a "what time is it" round trip is a few hundred tokens — fractions of a
cent per run. If cost ever matters, set `--model claude-haiku-4-5` (or
whichever is the smallest current model) in the driver scripts.

## When this doesn't work

- **`claude` complains about no API key.** You forgot `export
  ANTHROPIC_API_KEY=...`.
- **E2E-B fails at "SessionStart bootstrap didn't run".** Either the
  bootstrap shim is broken, or there's no GitHub release matching
  `plugin.json`'s version. Run `git tag` and check the latest release —
  E2E-B can only pass if a release exists for the manifest version.
- **`docker build` fails on `npm install -g @anthropic-ai/claude-code`.**
  Network issue, or the package was renamed. Confirm with `docker run --rm
  node:20-bookworm-slim npm view @anthropic-ai/claude-code version`.
- **Schema validation fails.** Almost always a real bug — capture wrote
  something the PLF spec says is invalid. Look at the offending record
  (the assertion script prints its path) and the ajv output for the field.

## CI integration

Not yet wired (deferred — see spec § "CI integration" for the plan).
The blocker is which secret-scoped contexts can run with the
`ANTHROPIC_API_KEY` repo secret; we'll likely gate behind a label for
PRs and run unconditionally on `main` post-merge plus pre-release-tag.
