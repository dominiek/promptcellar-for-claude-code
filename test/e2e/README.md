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
make test-e2e
```

That's it — assuming you've already authenticated `claude` on this host
(`claude login` for OAuth, or `export ANTHROPIC_API_KEY=…`), the suite
reuses whatever auth your host is already using. No separate test key
needed. Total wall clock ~60–90s on the first run, ~30–45s thereafter
(image cached).

To run a single scenario after the image is built:

```sh
bash test/e2e/run.sh   # runs both, with auth resolved as below
```

## How auth is resolved

[`test/e2e/run.sh`](./run.sh) walks this priority chain and uses the first
hit. The container then sees the resolved credential at the path Claude
Code's Linux build expects.

| Order | Source                                           | Mounted as / passed via                                         |
| ----- | ------------------------------------------------ | --------------------------------------------------------------- |
| 1     | `$ANTHROPIC_API_KEY` env var                     | `docker run -e ANTHROPIC_API_KEY` (no file written)             |
| 2     | macOS keychain entry `Claude Code-credentials`   | extracted to `mktemp` (mode 0600), mounted read-only at `/home/tester/.claude/.credentials.json`, removed on EXIT |
| 3     | `~/.claude/.credentials.json` on host            | mounted read-only at the same in-container path                 |

The keychain extract and the host file are both **read-only** mounts. The
tempfile is wiped via a shell `EXIT` trap whether the run succeeds, fails,
or is interrupted.

If none of the three resolve, the script prints what it tried and exits 2
with instructions.

### CI

In CI use the env-var path — set `ANTHROPIC_API_KEY` from a repo secret
scoped to a dedicated test account. Don't try to provision a keychain
entry on a runner; #1 is the path designed for that.

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
| `ANTHROPIC_API_KEY`| (none — falls through to keychain/file)  | Highest-priority auth source. Set this for CI.                   |
| `EXPECTED_PROMPT`  | `what time is it`                        | Sent to `claude -p` and asserted on the captured record.         |
| `PC_INSTALLER`     | `bash /repo/install/install.sh`          | E2E-A only. Set to `curl -fsSL https://get.promptcellar.io/claude-code \| sh` to test the deployed installer URL too. |
| `PC_MARKETPLACE`   | `dominiek/promptcellar-for-claude-code`  | E2E-B only. The GH repo registered as the marketplace.           |
| `PC_E2E_IMAGE`     | `promptcellar-e2e:latest`                | Image tag the build/run uses. Override to test a custom build.   |

## Cost & runtime

Each scenario fires one `claude -p` call. With Claude Code's default model,
a "what time is it" round trip is a few hundred tokens — fractions of a
cent per run. If cost ever matters, set `--model claude-haiku-4-5` (or
whichever is the smallest current model) in the driver scripts.

## When this doesn't work

- **`claude` complains about no API key inside the container.** Either no
  host auth was found (run `claude login` on the host, or set
  `ANTHROPIC_API_KEY`), or the keychain extract was blocked (unlock the
  keychain or grant access to the `security` CLI).
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
