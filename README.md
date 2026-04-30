# Promptcellar for Claude Code

A Claude Code plugin that captures every prompt you send to the agent and stores it in your repo as a structured, append-only log.

The captured data is the **human signal** that built the code — the questions, instructions, and corrections that shaped each commit. With it you can audit who asked the agent to do what, trace a commit back to the prompt that produced it, and keep that history under your team's control instead of in a vendor's database.

This repo is the **Claude Code plugin** that does the capture. The on-disk format is a separate open standard — see [`promptcellar-format/`](./promptcellar-format/) for the spec.

## What gets captured

For every prompt you submit, the plugin writes one JSONL record under `.prompts/` containing:

- The prompt text, your git author identity, the model and tool versions in use.
- A snapshot of git state at prompt time (branch, HEAD, dirty/clean).
- A summary of what the agent did in response: files touched, commits created, status (`completed` / `interrupted` / `errored`).
- Token usage, estimated cost, and wall-clock duration.

Records that match a `.promptcellarignore` pattern are replaced with an `excluded` stub so the timeline stays gap-free without leaking the prompt content.

Files land at:

```
.prompts/YYYY/MM/DD/HH/<session-id>.jsonl
```

…bucketed by session start time, one file per session, append-only. Because every session has a unique id, two branches can never write to the same file — merge conflicts in `.prompts/` are avoided by construction.

## Install

> **Status:** M3 complete. The installer (M2) and marketplace listing (M4) are in flight. For now you install from a local checkout.

```sh
make build
claude --plugin-dir ./plugin
```

That registers the plugin for the current Claude Code session. After M2 lands:

```sh
# bridge installer
curl -fsSL https://get.promptcellar.io/claude-code | sh

# eventually
claude plugin install promptcellar
```

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

Drop a `.promptcellarignore` at the repo root. Each line is a POSIX ERE pattern; if it matches the prompt text, the prompt is replaced with an `excluded` stub instead of being written.

```
id: secrets
(AWS_SECRET_ACCESS_KEY|GITHUB_TOKEN|OPENAI_API_KEY)

id: credential-shapes
(ghp_[A-Za-z0-9]{36}|sk-[A-Za-z0-9]{32,})
```

See [`promptcellar-format/SPEC.md` §4](./promptcellar-format/SPEC.md) for the full format.

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
cmd/                    # Hook binaries (one main.go each)
  pc-hook-session/
  pc-hook-prompt/
  pc-hook-tool/
  pc-hook-stop/
internal/               # Shared Go packages
  plf/                  # PLF record types + JSONL writer
  plfignore/            # .promptcellarignore parser/matcher
  capture/              # buffer-then-flush state machine
  transcript/           # Claude Code transcript adapter (versioned)
  gitsnap/              # cheap git metadata reads
  hookpayload/          # hook stdin parsing
  toolinfo/             # tool version detection
  pricing/              # token → cost table
  config/               # global/repo/personal layer resolution
plugin/                 # Claude Code plugin bundle
  .claude-plugin/plugin.json
  hooks/hooks.json
  bin/                  # built binaries (gitignored)
discovery-plugin/       # Throwaway forensic-dump plugin used during M0
test/                   # Integration test scripts
planning/               # Design + implementation plan
promptcellar-format/    # The PLF spec (separate project, vendored as a sibling)
```

## Building

Requires Go 1.26 or newer.

```sh
make build         # builds the four hook binaries into plugin/bin/
make test          # go test ./...
make clean         # rm -rf plugin/bin/
```

## Status

| Milestone | Scope                                                                                          | State    |
| --------- | ---------------------------------------------------------------------------------------------- | -------- |
| M0        | Discovery plugin; reverse-engineered Claude Code's hook payloads and transcript shape.         | done     |
| M1        | Hook binaries; buffer-then-flush state machine; six integration scenarios; <10ms cold start.   | done     |
| M2        | `curl \| sh` installer; slash commands; per-repo and global enable/disable.                    | next     |
| M3        | `PostToolUse` for `files_touched` and status; transcript polling for tokens/cost.              | done     |
| M4        | CI integration tests; signed binaries; marketplace listing.                                    | planned  |
| M5        | Optional MCP server for retrieval queries from inside Claude Code.                             | planned  |

[`planning/IMPLEMENTATION_PLAN.md`](./planning/IMPLEMENTATION_PLAN.md) has the detail behind each row.

## License

MIT.
