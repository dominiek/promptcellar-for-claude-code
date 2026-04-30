# Promptcellar for Claude Code

A Claude Code plugin that captures every prompt you send to the agent and stores it in your repo as a structured, append-only log.

The captured data is the **human signal** that built the code — the questions, instructions, and corrections that shaped each commit. With it you can audit who asked the agent to do what, trace a commit back to the prompt that produced it, and keep that history under your team's control instead of in a vendor's database.

This repo is the **Claude Code plugin** that does the capture. The on-disk format is a separate open standard — see [dominiek/promptcellar-format](https://github.com/dominiek/promptcellar-format) for the spec.

## Install

```sh
curl -fsSL https://get.promptcellar.io/claude-code | sh
```

This adds `dominiek/promptcellar-for-claude-code` as a Claude Code marketplace and installs the plugin. Open a new Claude Code session in any git repo to start capturing.

In-session:

```
/promptcellar:status     # confirm capture is on
/promptcellar:log 10     # last 10 captured prompts
/promptcellar:disable    # opt out for this repo (committed)
```

Building or installing from a checkout instead? See [Development](#development) below.

## What gets captured

For every prompt you submit, the plugin writes one JSONL record under `.prompts/` containing:

- The prompt text, your git author identity, the model and tool versions in use.
- A snapshot of git state at prompt time (branch, HEAD, dirty/clean).
- A summary of what the agent did in response: files touched, commits created, status (`completed` / `interrupted` / `errored`).
- Token usage, estimated cost, and wall-clock duration.

Records that match a `.promptcellarignore` pattern are replaced with an `excluded` stub so the timeline stays gap-free without leaking the prompt content.

Files land at:

```
.prompts/YYYY/MM/DD/<session-id>.jsonl
```

…bucketed by session start date (UTC), one file per session, append-only. Because every session has a unique id, two branches can never write to the same file — merge conflicts in `.prompts/` are avoided by construction.

## Default behaviour

- **On** in any directory where `git rev-parse --is-inside-work-tree` succeeds.
- **Off** everywhere else — no `.git/`, no capture.
- `.prompts/` and `.promptcellar/config.json` (the repo decision) are intended to be committed. `.promptcellar/state/` and `.promptcellar/config.local.json` are gitignored.

## Slash commands

All user-facing controls live inside Claude Code. There is no `promptcellar` binary on your `PATH`.

```
/promptcellar:status                        # resolved config + counts + .prompts location
/promptcellar:doctor                        # diagnose hook wiring, transcript parser, perms
/promptcellar:enable  [--for-me] [--global] # opt-in (default scope: repo, committed)
/promptcellar:disable [--for-me] [--global] # opt-out (default scope: repo, committed)
/promptcellar:log [N]                       # last N captured prompts in this repo
/promptcellar:version                       # plugin + transcript-adapter version
/promptcellar:uninstall                     # remove plugin entry; data is left intact
```

Enable/disable resolves through three layers, most-restrictive wins:

1. **Global** (`~/.promptcellar/config.json`) — kill switch for the whole machine.
2. **Repo** (`.promptcellar/config.json`, committed) — the team's decision.
3. **Personal** (`.promptcellar/config.local.json`, gitignored) — `--for-me` opt-out within a repo that has capture on.

## Excluding sensitive prompts

A built-in baseline catches well-known secret shapes by default — no setup required. When a prompt matches, it's replaced with an `excluded` stub: the timeline preserves the gap (you can see capture was skipped) but the prompt text never touches `.prompts/`.

### Built-in baseline (always on)

Compiled into the plugin. Pattern IDs surface in `excluded.pattern_id` so dashboards can bucket-count by source.

| Category | Catches | Example IDs |
| --- | --- | --- |
| **Cloud — AWS** | Access-key IDs (AKIA / ASIA / AGPA / etc.); env-style secret-access-key assignments | `aws-access-key-id`, `aws-secret-access-key-assignment` |
| **Cloud — GCP** | API keys (`AIza…`), OAuth tokens (`ya29.…`), service-account JSON markers | `gcp-api-key`, `gcp-oauth-token`, `gcp-service-account-json` |
| **Source forges** | GitHub classic + fine-grained PATs, OAuth / app / refresh tokens; GitLab PATs | `github-pat-classic`, `github-pat-fine-grained`, `gitlab-pat`, … |
| **AI providers** | Anthropic API keys (`sk-ant-…`), OpenAI keys (`sk-…` / `sk-proj-…` / `sk-svcacct-…`) | `anthropic-api-key`, `openai-api-key` |
| **Payment** | Stripe live/test/restricted/publishable, Twilio API key + account SID, SendGrid keys | `stripe-secret-live`, `twilio-api-key`, `sendgrid-api-key`, … |
| **Messaging** | Slack tokens (`xox[baprs]-…`) and webhook URLs, Discord bot tokens | `slack-token`, `slack-webhook`, `discord-bot-token` |
| **Generic** | JSON Web Tokens (three-segment), PEM private keys, DB / message-queue URLs with embedded credentials | `jwt`, `private-key-pem`, `db-url-with-password` |
| **SaaS** | npm + PyPI tokens; Mailgun, MailChimp, Datadog, Heroku keys | `npm-token`, `pypi-token`, `mailgun-api-key`, … |
| **Catch-all** | `API_KEY=…` / `SECRET=…` / `PRIVATE_KEY=…` style assignments with token-shaped values | `generic-secret-assignment` |

The full pattern set is in [`internal/plfignore/baseline.go`](./internal/plfignore/baseline.go). It's a stable contract: pattern IDs may grow but won't be silently renamed.

### Team additions: `.promptcellarignore` (committed)

Drop a `.promptcellarignore` at the repo root for team-specific deny patterns the baseline doesn't cover (internal API key prefixes, paths to security runbooks, customer identifiers, etc.). Each line is a POSIX ERE pattern; an `id: <name>` line above a pattern names it for `excluded.pattern_id`. Same syntax as `.gitignore`-style files for comments and blank lines.

```
id: internal-api
\bSECRET_INT_[A-Za-z0-9]{20,}\b

id: security-paths
\bsecurity/(runbooks|incident)\b
```

`.promptcellarignore` is **authoritative** — a team's deny rule always wins, regardless of the baseline or the allow file below.

### Override the baseline: `.promptcellarallow` (committed)

Same syntax as `.promptcellarignore`, but matches *whitelist* the prompt against baseline-driven exclusions. Use this when the baseline is too aggressive — typically because docs/fixtures contain placeholder values shaped like real tokens.

```
id: docs-examples
\bdocs/[^\s]+\.md\b

id: openapi-fixtures
\btest/fixtures/openapi/\b
```

`.promptcellarallow` only narrows the baseline; it cannot weaken `.promptcellarignore`.

### Resolution order

For each prompt:

1. **`.promptcellarignore` matches?** → exclude. Done. (Team rule is authoritative.)
2. **Baseline matches?**
   - **Also matches `.promptcellarallow`?** → capture normally. (Allow overrides the baseline.)
   - **Otherwise** → exclude.
3. **Otherwise** → capture.

See [`promptcellar-format` SPEC §4](https://github.com/dominiek/promptcellar-format/blob/main/SPEC.md) for the on-disk shape of `excluded` records.

## Reading your captured data

It's just JSONL. `jq` is enough:

```sh
# Last 10 prompts across all branches
find .prompts -name '*.jsonl' -exec cat {} + \
  | jq -s 'sort_by(.timestamp) | .[-10:] | .[] | {timestamp, prompt}'

# Total cost on a branch
find .prompts -name '*.jsonl' -exec cat {} + \
  | jq -s '[.[] | select(.git.branch == "feat/auth-rewrite") | .enrichments.cost_usd // 0] | add'

# Prompts that touched a path
find .prompts -name '*.jsonl' -exec cat {} + \
  | jq -r 'select(.outcome.files_touched // [] | any(startswith("server/auth/"))) | .prompt'
```

The eventual MCP server (M5) will give Claude itself read access to the same data. The plain files come first; queryability is additive.

## How it works

Capture is driven by Claude Code plugin hooks:

| Event                | Hook               | Action |
| -------------------- | ------------------ | ------ |
| Session opens        | `SessionStart`     | Cache git author + tool version; flush orphan state from previously-crashed sessions as `interrupted`. |
| You submit a prompt  | `UserPromptSubmit` | Match `.promptcellarignore`; mint a record id; snapshot git; buffer to `.promptcellar/state/<session>.json`. |
| Agent runs a tool    | `PostToolUse`      | For `Edit/Write/MultiEdit/NotebookEdit`, append the path to the active prompt's buffered `files_touched`. |
| Agent finishes       | `Stop`             | Synchronously flush the full PLF record to JSONL: outcome summary from `last_assistant_message`, token and cost enrichment by briefly polling the transcript. |

Each hook is a small Go binary in `plugin/bin/`, invoked via plugin-relative paths declared in the manifest. Cold start is under 10ms.

A deeper walkthrough — buffer-then-flush rationale, transcript adapter strategy, crash recovery — lives in [`planning/IMPLEMENTATION_PLAN.md`](./planning/IMPLEMENTATION_PLAN.md).

## Repository layout

```
cmd/                    # Six binaries, one main.go each
  pc-hook-session/      #   SessionStart hook
  pc-hook-prompt/       #   UserPromptSubmit hook
  pc-hook-tool/         #   PostToolUse + PostToolBatch hooks
  pc-hook-stop/         #   Stop hook (synchronous flush)
  pc-cli/               #   user-facing CLI behind the slash commands
  pc-mcp/               #   stdio MCP retrieval server
internal/               # Shared Go packages
  plf/                  #   PLF record types + JSONL writer
  plfignore/            #   .promptcellarignore parser/matcher
  plfread/              #   read existing .prompts/ records (for CLI + MCP)
  capture/              #   state machine + flush logic
  transcript/           #   Claude Code transcript adapter (versioned)
  gitsnap/              #   cheap git metadata reads
  hookpayload/          #   hook stdin parsing
  toolinfo/             #   tool version detection
  pricing/              #   token → cost table
  config/               #   global/repo/personal layer resolution
plugin/                 # Claude Code plugin bundle
  .claude-plugin/plugin.json
  hooks/hooks.json
  commands/*.md         #   slash commands (AI-driven shells over pc-cli)
  .mcp.json             #   pc-mcp registration
  bin/                  #   built binaries (gitignored)
.claude-plugin/         # Marketplace manifest (this repo IS a marketplace)
  marketplace.json
install/                # `curl | sh` and dev-mode installers
scripts/                # Release flow: bump-version, release, marketplace-publish
.github/workflows/      # ci.yml (PRs) + release.yml (tags)
discovery-plugin/       # Throwaway forensic-dump plugin used during M0
test/                   # Integration test scripts
planning/               # Design + implementation plan
                        # The PLF spec lives at https://github.com/dominiek/promptcellar-format
```

## Development

Requires Go 1.26 or newer. The PLF JSON Schema used by the integration suite is vendored at `test/fixtures/plf-1.schema.json`; the upstream lives at [dominiek/promptcellar-format](https://github.com/dominiek/promptcellar-format).

```sh
git clone https://github.com/dominiek/promptcellar-for-claude-code.git
cd promptcellar-for-claude-code

make build               # all six binaries (4 hooks + pc-cli + pc-mcp) into plugin/bin/
make test                # go test ./...
make test-all            # unit + M1 + M2 + M3 integration suites (~30s total)
make cross-build         # darwin/linux/windows × arm64/x64 → dist/<platform>/
make clean               # rm -rf plugin/bin/ dist/
```

### Installing from source

Two options for working off a local checkout.

**Per-session (no marketplace registration).** Pass `--plugin-dir` when launching Claude Code; the flag applies for that one session only:

```sh
make build
claude --plugin-dir ./plugin
```

**Persistent dev install.** Registers this checkout as a local marketplace and installs the plugin from it, so subsequent `claude` invocations pick it up without flags:

```sh
bash install/dev-install.sh
```

The script runs `make build`, validates the manifest, calls `claude plugin marketplace add ./` against this repo, and installs `promptcellar@promptcellar`. Uninstall with `/promptcellar:uninstall` from inside Claude Code (or `claude plugin uninstall promptcellar@promptcellar` from a shell).

> Note: direct cache-dir copies bypassing the `claude plugin marketplace add` + `claude plugin install` flow are silently ignored — Claude Code marks unknown-marketplace entries as `disabled`. Always use `dev-install.sh` (or `--plugin-dir` for ad-hoc tests).

## Cutting a release

A release publishes cross-compiled tarballs (darwin/linux/windows × arm64/x64) to GitHub Releases. The marketplace publication is a separate step so updating the public Anthropic listing is an explicit, conscious action.

### 1. Cut and tag

```sh
bash scripts/release.sh 0.4.0
```

The script:

- Verifies the working tree is clean and you're on `main`.
- Calls `scripts/bump-version.sh 0.4.0` (patches `plugin/.claude-plugin/plugin.json` plus the `Version` / `serverVersion` consts in `cmd/pc-cli/main.go` and `cmd/pc-mcp/main.go`).
- Runs `make test-all`.
- Commits the bump, creates the `v0.4.0` annotated tag, and prompts before pushing.

When the tag is pushed, [`.github/workflows/release.yml`](./.github/workflows/release.yml) cross-compiles the six binaries for five platforms and uploads tarballs to a GitHub Release.

To bump versions without cutting a release (e.g. during a feature branch), use the underlying script directly:

```sh
bash scripts/bump-version.sh 0.4.0-rc.1
```

### 2. Marketplace publication (separate, optional)

After the GitHub Release is live, *only if* you want this version on the public Anthropic marketplace (so users worldwide can `claude plugin install promptcellar` without first registering this repo as a marketplace):

```sh
bash scripts/marketplace-publish.sh
```

It prints the marketplace entry and PR instructions for [`anthropics/claude-plugins-official`](https://github.com/anthropics/claude-plugins-official). You only need to do this once — subsequent releases reuse the same entry (it tracks the `main` branch ref).

## Status

| Milestone | Scope                                                                                          | State |
| --------- | ---------------------------------------------------------------------------------------------- | ----- |
| M0        | Discovery plugin; reverse-engineered Claude Code's hook payloads and transcript shape.         | done  |
| M1        | Hook binaries; buffer-then-flush state machine; six integration scenarios; <10ms cold start.   | done  |
| M2        | `curl \| sh` installer; slash commands; per-repo and global enable/disable; `.promptcellarignore`. | done  |
| M3        | `PostToolUse` for `files_touched` and status; transcript polling for tokens/cost.              | done  |
| M4        | CI integration tests; cross-build for 5 platforms; release workflow.                           | done  |
| M5        | MCP retrieval server: `promptcellar.{search,log,touched,session}`.                             | done  |

[`planning/IMPLEMENTATION_PLAN.md`](./planning/IMPLEMENTATION_PLAN.md) has the detail behind each row.

## License

MIT.
