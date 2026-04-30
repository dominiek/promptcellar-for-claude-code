# Promptcellar for Claude Code — Implementation Plan

**Status:** Draft 2 (2026-04-29) — for review.
**Scope:** Claude Code only. Other agents (Cursor, Aider, Codex) deferred.

## TL;DR

Ship a **Claude Code plugin** that bundles three things:

1. **Hooks** — `UserPromptSubmit`, `Stop`, `PostToolUse`, `SessionStart`, `SessionEnd` — for capture.
2. **Slash commands** (`/promptcellar`) — the *only* user-facing surface. No CLI on PATH.
3. **MCP server** (optional, follow-up) — for retrieval queries from the agent.

Everything lives inside the Claude Code plugin sandbox: hook binaries are bundled in the plugin directory and invoked via plugin-relative paths, never PATH. Users never run a `promptcellar` command in their shell.

Distribution: just two paths — the **Claude Code plugin marketplace** when ready, and a `**curl | sh` installer** as the bridge until then. Both produce the same on-disk artifact: a Claude Code plugin registered in `~/.claude/settings.json`.

Capture defaults: ON in any directory with `.git/`. OFF otherwise; the *only* way to turn it on in a non-git directory is `/promptcellar enable`.

The "deeper integration" is reading Claude Code's session transcript file at `Stop` time — the hook payload tells us its path (`transcript_path`), but the file's internal JSON shape is undocumented, so transcript-derived fields (`model.`*, `enrichments.*`, full `outcome.summary`) are best-effort with a versioned adapter. Before locking the design we run a discovery pass (§5.1) to pin down what is actually accessible.

---

## 1. Capture pipeline

The PLF spec (§2.1) mandates append-only files, so we cannot write a "pending" record and patch it later. We buffer per-session state in a private sidecar and flush one canonical record per prompt at `Stop`.


| Event                     | Hook               | Action                                                                                                                                                                                                                                                                          |
| ------------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Session opens             | `SessionStart`     | If `stdin.model` is present (`source:"startup"` interactive only), cache it. Else defer model resolution to flush time. Cache git author + tool version; resolve enable/disable config; flush orphan state files belonging to **other** session_ids as `interrupted` (this session's own state — if any from a prior `--resume`-style re-init — is preserved). |
| User submits a prompt     | `UserPromptSubmit` | If a previously-buffered prompt has no `Stop` yet → flush it as `interrupted` (preemption). Match `.promptcellarignore`; if matched, write an `excluded` stub. Otherwise mint a new record `id`, snapshot git, and buffer to state.                                              |
| Agent runs a tool         | `PostToolUse`      | If tool is `Edit/Write/MultiEdit/NotebookEdit`, append the file path to the active prompt's buffered `files_touched`.                                                                                                                                                            |
| Agent finishes responding | `Stop`             | **Synchronously flush the full PLF record** to JSONL using `last_assistant_message` (from the Stop payload itself — no transcript parse needed for `outcome.summary`). For M3+ enrichment, briefly poll the transcript inside this hook for the just-finished assistant record. |

Design note: **flush-at-Stop**, not deferred-flush. We flush synchronously inside the Stop hook so `.prompts/` is always current relative to the user's commit/push activity — sessions can stay open for days, but every completed prompt is on disk by the time the user touches their git history. M0 found that the just-completed assistant record isn't in the transcript at Stop time (persisted async); M3 will handle that by polling the transcript briefly inside Stop (still synchronous, sub-second typical) before flushing the enriched record. Stop is a natural place to absorb a small latency — the agent has already stopped responding by then.

Notes from M0:
- Claude Code provides no `SessionEnd` hook. Truly crashed sessions (no `Stop` ever fires) are detected on the *next* `SessionStart` by scanning for orphan state files and flushing them as `interrupted`.


**State file:** `.promptcellar/state/<session-id>.json` — gitignored, owned by exactly one process. Contains the buffered prompt + accumulating `files_touched` + session-level git/author cache.

**JSONL file:** `.prompts/YYYY/MM/DD/HH/<session-id>.jsonl` — public PLF format, append-only, single-writer per file.

**Crash recovery:** state files older than the running PID's session are scanned at next `SessionStart` and flushed as `interrupted` records. The "lost work" window is bounded to an active in-flight prompt, and even that becomes a recorded interrupted record — not silent data loss.

---

## 2. Installation

Two paths only:

### 2.1 Claude Code plugin marketplace (target steady state)

```
claude plugin install promptcellar
```

Single command, no other prerequisites. Claude Code handles on-disk placement, version pinning, and updates. This is the path we want every user on once the plugin marketplace is the right shape for our needs. Until then, marketplace listing logistics shouldn't block adoption — hence the curl installer below.

### 2.2 Curl installer (bridge, also the v1 critical path)

```sh
curl -fsSL https://get.promptcellar.io/claude-code | sh
```

What it does:

1. Detects platform (`darwin-arm64`, `darwin-x64`, `linux-x64`, `linux-arm64`, `windows-x64`).
2. Downloads the plugin bundle (manifest + bundled hook binaries + slash command files + optional MCP server) into `~/.claude/plugins/promptcellar/`.
3. Registers the plugin in `~/.claude/settings.json` under user scope.
4. Drives `/promptcellar doctor` once on the next session — green/red report on hook wiring, transcript readability, write permissions.

The installer **does not** put anything on the user's PATH, modify shell rc files, or install runtimes. The only on-disk footprint outside `~/.claude/` is the `.prompts/` and `.promptcellar/` directories that capture creates inside repos.

Same end-state as the marketplace path: a registered Claude Code plugin. From the user's seat, *all* interaction happens in-session via slash commands; the curl install is "one and done."

### 2.3 Why a compiled binary

The `UserPromptSubmit` hook fires on every prompt and must add <50ms. Go or Rust startup hits that easily; Node startup (~150–250ms cold) is borderline. Also: zero runtime dependency on the user's box. The bundled hook binaries live inside the plugin directory; hooks invoke their siblings via plugin-relative paths declared in the plugin manifest — never PATH.

### 2.4 Uninstall

`/promptcellar uninstall` from inside Claude Code, or `claude plugin uninstall promptcellar` if installed via the marketplace. Both remove the plugin entry. Captured `.prompts/` data is left in place — uninstall does not delete history.

---

## 3. Enable / disable per repo

Promptcellar tracks prompts at the **repo level**. A repo either captures or it doesn't — that's the primary decision, made once and shared with the team via committed config. The repo is the unit. Individuals who don't want their own prompts captured use a separate personal flag, but they cannot unilaterally turn the repo off for everyone.

**Default:** ON in any directory where `git rev-parse --is-inside-work-tree` succeeds. OFF otherwise. Non-git directories are out of scope — there's no committed `.prompts/` to write to, no team to share captures with, and no place for the repo decision to live.

### 3.1 Repo-level (committed)

`/promptcellar enable` and `/promptcellar disable` operate at the repo level. They write `.promptcellar/config.json`, which is committed. After commit + push, every collaborator who pulls inherits the decision on their next session.

These are the commands that travel with the code. If the team decides this repo doesn't track prompts, the decision lives in `.promptcellar/config.json` and applies to everyone.

### 3.2 Personal flag (gitignored)

`/promptcellar disable --for-me` suppresses capture of *your* prompts in the current repo, even if the repo has capture on. Writes `.promptcellar/config.local.json`, which is gitignored. Teammates' captures continue normally.

`/promptcellar enable --for-me` removes the personal disable.

The `--for-me` flag is opt-out only with respect to the repo decision: if the team has disabled the repo, you can't `--for-me enable` your way around it. That'd write your prompts into a `.prompts/` folder the team's committed config has said shouldn't exist — confusing, and a source of accidental commits.

### 3.3 Global kill-switch (machine-level)

`/promptcellar disable --global` writes `~/.promptcellar/config.json` with `{ "enabled": false }` and turns capture off across every repo on this machine. Useful for "I'm working on something sensitive today, just stop." Re-enable with `/promptcellar enable --global`.

### 3.4 Resolution order

Most-restrictive wins. Capture happens iff none of the three layers say "off":

1. Global disabled → no capture anywhere on this machine.
2. Repo disabled (committed) → no capture in this repo.
3. Personal disabled (`--for-me`) → no capture for you in this repo.
4. Otherwise → capture.

`/promptcellar status` shows the resolved state plus which layer flipped it, so it's never a mystery.

### 3.5 File map

```
.promptcellar/config.json         # repo-level decision (committed)
.promptcellar/config.local.json   # personal opt-out (gitignored)
.promptcellar/state/              # runtime state (gitignored)
~/.promptcellar/config.json       # machine-wide kill-switch
```

The `SessionStart` hook adds `.promptcellar/state/` and `.promptcellar/config.local.json` to `.git/info/exclude` (per-clone, not committed) on first capture. We don't touch `.gitignore` itself. The `.promptcellar/config.json` file is meant to be in git, so it stays out of the exclude list.

All three layers are re-read on each `UserPromptSubmit` so toggles take effect on the next prompt without restarting the session.

---

## 4. Metadata capture — what hooks give us vs. what we need

Field-by-field source map. The "needs transcript" rows are the ones the M0 discovery pass (§5.1) is going to validate.


| PLF field                                  | Source                                                                                                  | Confidence              |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------- | ----------------------- |
| `version`, `id`, `session_id`, `timestamp` | Generated locally                                                                                       | trivial                 |
| `author.email/name`                        | `git config user.email` / `user.name`                                                                   | trivial                 |
| `author.id`                                | `git config user.signingkey` fingerprint, when set                                                      | easy                    |
| `tool.name`                                | Hardcoded `"claude-code"`                                                                               | trivial                 |
| `tool.version`                             | Parse `AI_AGENT` env var (`claude-code/<X.Y.Z>/harness` → `<X.Y.Z>`). Reliably set in hook subprocesses. `CLAUDE_CODE_EXECPATH` is NOT in hook env (M1 finding — earlier docs were wrong). | confirmed M1 |
| `prompt`                                   | `UserPromptSubmit` hook stdin                                                                           | documented              |
| `git.branch`                               | Transcript records carry `gitBranch` on every line — read once, no shell-out                            | confirmed M0            |
| `git.head_commit / dirty`                  | `git rev-parse HEAD` / `git status --porcelain`                                                         | trivial                 |
| `parent.prompt_id`                         | Track previous prompt's `id` in session state                                                           | trivial                 |
| `outcome.files_touched`                    | `PostToolUse` hook on Edit/Write/MultiEdit/NotebookEdit                                                 | documented              |
| `outcome.commits`                          | `git log --since=<prompt_ts> HEAD`                                                                      | trivial                 |
| `outcome.status`                           | `Stop` seen → `completed`; new `UserPromptSubmit` without prior `Stop` → `interrupted`; any `PostToolBatch.tool_calls[].tool_response` matching error patterns (`PostToolUse` is skipped on tool errors — confirmed M0 v2) → `errored` | confirmed M0 v2 |
| `outcome.summary`                          | `Stop.stdin.last_assistant_message` truncated to ≤500 chars — **no transcript parse needed**            | confirmed M0            |
| `model.provider`                           | Hardcoded `"anthropic"`                                                                                 | trivial                 |
| `model.name`                               | `SessionStart.stdin.model` when present (interactive `source:"startup"`); otherwise the most recent `assistant.message.model` from the transcript at flush time (headless `sdk-cli` mode and `source:"resume"` SessionStarts omit `stdin.model`) | confirmed M0 v2         |
| `model.version`                            | null (no separate version pin)                                                                          | trivial                 |
| `enrichments.tokens.*`                     | **Aggregated from transcript** between this prompt's slot and the next                                  | needs transcript        |
| `enrichments.cost_usd`                     | Computed from tokens × published Anthropic prices (table bundled, refresh-on-update)                    | derived from transcript |
| `enrichments.duration_ms`                  | `UserPromptSubmit` ts → `Stop` ts                                                                       | trivial                 |


### 4.1 The "unofficial" integration, demystified

The hook payload includes `transcript_path` — a documented contract Claude Code provides. Locating the transcript is official. What is *not* documented is the JSON shape of records inside that transcript. We depend on it for `model.*` and `enrichments.*`.

Mitigations:

- **Transcript-derived fields are optional in PLF.** If parsing fails, omit those fields and write the rest of the record. Capture never breaks because of a transcript schema change.
- **Versioned adapters.** A small `internal/transcript/` package detects the Claude Code version (from `tool.version`) and selects an adapter. New CC releases get a new adapter; old ones keep working.
- `**/promptcellar doctor`** runs a parse self-test against the user's most recent transcript and warns loudly if shape has drifted from what our adapter expects.
- **Conservative parser.** No assumptions beyond what we strictly need; ignore unknown fields; tolerate field-name churn via per-version adapter.

This is the deepest integration we need. We are not patching Claude Code, intercepting its IPC, or doing anything that'd survive a `claude` upgrade poorly — we're reading a file Claude Code already writes for us and tells us where to find. But because the shape isn't a public API, we *prove* what's accessible before locking the design — see §5.

---

## 5. Integration test harness

Two layers, both ship as part of the v1 work.

### 5.1 Layer 1: data-discovery plugin (M0)

**Status: built, awaiting harvest.** Lives at `discovery-plugin/` in this repo.

It's a throwaway Claude Code plugin that registers seven hook entry points (`SessionStart`, `UserPromptSubmit`, `UserPromptExpansion`, `PreToolUse`, `PostToolUse`, `PostToolBatch`, `Stop`) and does one thing: dumps every hook invocation's stdin payload + relevant env vars + a snapshot of the referenced `transcript_path` to a per-session forensic log at `~/.promptcellar-discovery/<session-id>/`.

```
~/.promptcellar-discovery/<session-id>/
  0001-SessionStart.json
  0001-SessionStart.transcript.jsonl
  0002-UserPromptSubmit.json
  0002-UserPromptSubmit.transcript.jsonl
  0003-PreToolUse.json
  ...
```

Each `<seq>-<event>.json` contains: parsed stdin, raw stdin (if unparseable), `CLAUDE_*`/`CC_*`/`ANTHROPIC_*` env vars, argv, cwd, pid, wall timestamp. The matching `<seq>-<event>.transcript.jsonl` is a copy of the file at `transcript_path` at that exact moment — so we can see how the transcript grows hook-by-hook.

**Install:** `claude --plugin-dir ./discovery-plugin` from the repo root. The plugin is active only for that session.

**Harvest checklist:** see `discovery-plugin/README.md`. Run real CC sessions exercising normal prompts, file edits, git commands, errors, mid-response interrupts, secret-shaped prompts, slash-command invocations.

**Outputs:**

- `planning/HOOK_PAYLOAD_REFERENCE.md` — produced by reading the dumps; pins down hook stdin schemas, transcript JSONL shape, env vars, anything that diverged from the docs.
- `test/fixtures/discovery/cc-<version>/` — redacted dumps used as inputs to Layer 2 unit tests in M3+.

Until the harvest + reference doc land, M1+ production code remains best-effort.

### 5.2 Layer 2: end-to-end automated tests (post-M1)

A test runner that:

1. Spins up a fresh repo in a temp dir (`git init`, one commit so HEAD exists).
2. Installs the Promptcellar plugin into a sandboxed Claude Code config (a separate `~/.claude/` rooted at the temp dir, via `CLAUDE_CONFIG_DIR` or the equivalent — to be confirmed in M0).
3. Drives Claude Code in headless mode (`claude -p "<prompt>"` or whatever invocation M0 confirms) with a fixed prompt set.
4. Reads the resulting `.prompts/...` JSONL files.
5. **Validates each record against the PLF JSON Schema** at `promptcellar-format/schemas/plf-1.json`.
6. Asserts per-scenario expectations:
  - normal prompt → `prompt` set, `outcome.status = "completed"`, `model.`* set, `enrichments.tokens.*` set, all required PLF fields present.
  - prompt that edits a file → `outcome.files_touched` includes the right path.
  - prompt that creates a commit → `outcome.commits` non-empty.
  - prompt matching `.promptcellarignore` → `excluded` stub written, no `prompt`, no `outcome`.
  - simulated crash mid-prompt (kill the agent process) → next session emits an `interrupted` record on recovery.

Test scenarios live as fixtures under `test/scenarios/<name>/{prompt.txt,expected.json}`; the runner produces a junit-style report and runs in CI.

### 5.3 Manual integration checklist

For releases, a short markdown checklist a developer walks through with a real Claude Code session:

1. Fresh install via curl script. `/promptcellar status` shows enabled.
2. Submit "what time is it" — confirm a captured record appears.
3. Submit a prompt containing `GITHUB_TOKEN=ghp_...` — confirm an `excluded` stub appears, no plaintext.
4. Submit a prompt that edits a file. Confirm `files_touched`.
5. Submit a prompt, then commit. Confirm `commits`.
6. `/promptcellar disable`, submit a prompt, confirm nothing captured. Re-enable.
7. Kill the agent mid-response. Reopen. Confirm an `interrupted` record on next session.
8. `/promptcellar doctor` returns green.

Catches things automated tests miss (e.g. UX rough edges in slash commands) and serves as the release gate for v1.

---

## 6. PLF writer & ignore matcher

**JSONL writer:** single writer per file (one process owns one session), so no locking. Atomic single-`write()` append for line < `PIPE_BUF`. For oversized records, write to temp + `rename()` over. Stable key ordering for diff readability. UTF-8 no BOM. `\n` terminator. Hour-bucket dir created with `mkdir -p` on every flush (cheap, idempotent).

`**.promptcellarignore`:** loaded once at `SessionStart`, recompiled on `stat` change. Patterns are POSIX ERE with case-insensitive default per spec §4.1. On match → write `excluded` stub immediately at `UserPromptSubmit` time (we have everything we need; no waiting for `Stop`). Stub includes `pattern_id` if the matched pattern declared one.

---

## 7. Repository layout

```
promptcellar-for-claude-code/
  cmd/
    pc-hook-prompt/           # UserPromptSubmit fast path
    pc-hook-stop/             # Stop: parse transcript, flush record
    pc-hook-tool/             # PostToolUse / PostToolBatch: append files_touched
    pc-hook-session/          # SessionStart: cache + orphan recovery
    pc-slash/                 # Backend for /promptcellar slash commands (shelled out from markdown)
  discovery-plugin/           # M0 forensic-dump plugin (Python, throwaway)
    .claude-plugin/plugin.json
    hooks/{hooks.json,dump.py}
    README.md
  internal/
    plf/                      # PLF record types, marshaller, validator
    plfignore/                # .promptcellarignore parser/matcher
    capture/                  # state-file format, flush logic
    transcript/               # Claude Code transcript adapter (versioned)
    gitsnap/                  # cheap git metadata reads
    ccplugin/                 # plugin manifest writer + settings.json edits
    config/                   # global + per-repo enable/disable resolution
  plugin/
    plugin.json               # Claude Code plugin manifest (hooks + slash commands + optional MCP)
    commands/
      promptcellar.md         # /promptcellar slash command spec
  install/
    install.sh                # the curl|sh installer (only distribution script we ship)
  test/
    fixtures/
      discovery/              # output of M0 data-discovery sessions, redacted
      scenarios/              # end-to-end test scenarios
    runner/                   # automated integration test harness
  planning/
    DESIGN.md
    PLF_STANDARD_DESIGN.md
    HOOK_PAYLOAD_REFERENCE.md # produced by M0
    IMPLEMENTATION_PLAN.md    # this file
```

Compiling each hook as a separate small binary (vs. one fat binary with subcommands) keeps the per-prompt fast path tiny. None of these binaries land on the user's PATH — they live inside the plugin directory and are invoked via plugin-relative paths declared in the plugin manifest's hook entries.

---

## 8. Slash command surface

The full user-facing surface — no userland CLI, no shell config to edit:

Plugin name: `promptcellar`. Claude Code namespaces plugin commands as `<plugin>:<command>` — confirmed M0 v2 — so the recorded `prompt` text on invocation is the namespaced form. Users can typically type the bare form (`/status`) and CC resolves it.

```
/promptcellar:status                          # resolved config with reasons; counts; .prompts location
/promptcellar:doctor                          # diagnose hook wiring, transcript parser, perms
/promptcellar:enable  [--for-me] [--global]   # opt-in (default scope: repo, committed)
/promptcellar:disable [--for-me] [--global]   # opt-out (default scope: repo, committed)
/promptcellar:log [N]                         # show last N captured prompts in this repo (git-log style)
/promptcellar:version                         # plugin + transcript-adapter version
/promptcellar:uninstall                       # remove the plugin entry; data is left intact
```

`/promptcellar enable`/`disable` change the repo's committed decision by default. `--for-me` flips your personal layer only (gitignored). `--global` flips the machine-wide kill-switch. See §3.4 for resolution order.

---

## 9. MCP server (follow-up)

A small read-only MCP server bundled in the same plugin (off by default in plugin manifest). Exposes:

- `promptcellar.search(query)` — fuzzy match over prompt text in `.prompts/`.
- `promptcellar.log(n)` — last N prompts in this repo.
- `promptcellar.touched(path)` — prompts whose `outcome.files_touched` includes `path`.
- `promptcellar.session(id)` — all prompts in a session.

Lets the agent itself query its own history ("what prompts touched `server/auth/` last week?"). Strictly additive — not on the M1–M4 critical path.

---

## 10. Milestones


| Milestone | Scope                                                                                                                                                                                                                                                                                                                                                                                                                   | Estimate  |
| --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- |
| **M0**    | ✅ **Complete.** Discovery plugin built (`discovery-plugin/`), harvested in a real CC 2.1.123 session (88 hook fires, 6 prompts, slash-command + secret-shaped + interrupt scenarios), reference doc at `planning/HOOK_PAYLOAD_REFERENCE.md`. Major findings folded into §1 capture pipeline and §4 metadata table. M1 contract is now locked. | ✅ done |
| **M1**    | ✅ **Complete.** Three Go binaries (`pc-hook-session`, `pc-hook-prompt`, `pc-hook-stop`) compiled into `plugin/bin/`. Buffer-then-flush state machine with deferred flush at next-hook-after-Stop. Six integration scenarios pass (happy / interrupted / orphan recovery / non-git / empty repo / concurrent sessions). Every emitted record validates against `plf-1.schema.json`. Cold start <10ms (target was <50ms). Install: `claude --plugin-dir ./plugin`. | ✅ done |
| **M2**    | ✅ **Complete.** `pc-cli` binary with `status / enable / disable / log / doctor / version / uninstall` subcommands. Layered config resolver (personal / team / global) with most-restrictive-wins semantics. `.promptcellarignore` POSIX-ERE matcher with `id:` labelling, integrated into `pc-hook-prompt` to write `excluded` stubs immediately on match. Slash commands at `plugin/commands/*.md` (AI-driven shells over `pc-cli` via Bash). `.claude-plugin/marketplace.json` at the repo root registers the plugin as a one-entry local marketplace. **Both installers go through `claude plugin marketplace add` + `claude plugin install`** — direct file-copy + `installed_plugins.json` editing leaves entries marked disabled (CC validates marketplace registration). Six M2 integration scenarios pass. | ✅ done |
| **M3**    | ✅ **Complete.** Added `pc-hook-tool` (registered for `PostToolUse` + `PostToolBatch`) for `outcome.files_touched` (Edit/Write/MultiEdit/NotebookEdit only, repo-relative dedup'd) and `outcome.status="errored"` (heuristic on PostToolBatch tool_response). Stop hook now polls transcript (≤3s) for the just-finished assistant record before flushing, populating `enrichments.tokens.{input,output,cache_read,cache_write}` and `enrichments.cost_usd` (via `internal/pricing/`). `enrichments.duration_ms` = Stop ts − UserPromptSubmit ts. `outcome.commits` via `<head_at_prompt>..HEAD` log query (precise; not affected by `--since` second-resolution). Five new integration scenarios pass; all M1 scenarios still pass. | ✅ done |
| **M4**    | ✅ **Complete (CI + cross-build).** GitHub Actions workflows: `ci.yml` runs unit + M1 + M3 + M2 integration tests on push/PR (macOS + Ubuntu). `release.yml` cross-compiles for darwin-{arm64,x64}, linux-{x64,arm64}, windows-x64 on tag push and uploads tarballs to GitHub Releases. `make cross-build` does the same locally — verified produces 6×5=30 binaries. Skipped (deferred): macOS notarization (needs an Apple developer cert) and marketplace listing (per earlier guidance — focus on end-to-end first). | ✅ done |
| **M5**    | ✅ **Complete.** `pc-mcp` binary speaks JSON-RPC 2.0 over stdio. Tools: `promptcellar.search(query, limit?)`, `promptcellar.log(limit?)`, `promptcellar.touched(path, limit?)`, `promptcellar.session(session_id)`. Registered via `plugin/.mcp.json` using `${CLAUDE_PLUGIN_ROOT}/bin/pc-mcp`. Backed by a shared `internal/plfread/` package (used by both `pc-cli log` and `pc-mcp`). Two MCP integration scenarios pass — `tools/list` exposes all four tools with proper input schemas; `tools/call` returns the matching record set; unknown tool name yields an RPC error. | ✅ done |


Total to a useful v1 (through M4): ~4–5 weeks of focused work.

---

## 11. Risks & known unknowns

- **Plugin manifest drift.** Claude Code's plugin system may evolve. Mitigated by keeping the manifest minimal and treating M0's discovery output as the integration contract we adapt over time.
- **Transcript schema drift.** Covered in §4.1 — versioned adapters + `/promptcellar doctor` self-test + Layer-2 CI tests catch regressions.
- **Hook performance.** Need <50ms `UserPromptSubmit`. Measured on every release; CI gate.
- **Windows.** Hooks must work cross-platform. The append-write needs the Windows code path (`FILE_APPEND_DATA`). `transcript_path` location may differ. Plan for it; need a Windows session in the M0 discovery sweep.
- **Empty repos / new repos.** `git rev-parse HEAD` fails before the first commit. Handle: emit record without `git.head_commit` rather than skipping.
- **Slash commands are AI-driven.** Confirmed in M0: a slash command's markdown body is *instructions to Claude*, not a shell script. `/promptcellar enable` either tells Claude to use Write/Bash itself, or — cleaner — declares `allowed-tools: ["Bash"]` and shells out to a small script we ship under the plugin. Either way, repo write access works (hooks and tools share the same permission context).
- **Repo-level decisions need pushing.** `/promptcellar disable` writes `.promptcellar/config.json` locally; until the user commits and pushes, teammates won't inherit the change. The slash command should remind them to commit + push and offer to do it.
- **`.prompts/` must stay current with git activity.** Long-lived CC sessions (days) followed by a commit/push must not leave the last prompt unwritten. **Resolution: flush-at-Stop** (synchronous, inside the Stop hook). For M3 enrichment, the Stop hook briefly polls the transcript for the just-finished assistant record (typically sub-second). This adds a small latency to Stop but the agent has already stopped responding, so it's imperceptible.
- **`PostToolUse` skipped on tool errors.** Confirmed M0 v2: when a tool errors out without producing a normal result (e.g. `Read` of a non-existent file), `PostToolUse` does not fire. `PostToolBatch` always fires and contains the error response. Use `PostToolUse` for `outcome.files_touched` (correctness — failed writes shouldn't count), and `PostToolBatch` for `outcome.status` error detection (completeness — every tool call surfaces here, success or error).

---

## 12. Open questions for you

1. **M0 scope and time-box.** I want 1–3 days of pure discovery work — no production code, just the data-dump plugin and a written reference document. **OK to start there? Yes**
2. **Non-git directories: out of scope.** Capture only happens in git repos. There's no slash command to enable Promptcellar in a directory without `.git/` — by design, since there'd be no committed config to anchor the repo decision and no shared `.prompts/` for the captures to live in. Good? Yes good for now
3. **Outcome summary quality.** "Last assistant message truncated to 500 chars" is cheap and works for v1. Spec §3.8 hints at *summaries*. Do we want an LLM-summarized version (call Claude API at `Stop` time, ~100ms, ~$0.001 per prompt) as a follow-up? Lean: yes for M3 if M0 confirms cost is bearable. No we cannot do any LLM calls so the outcome summary has to be whatever we can have the MCP give us or what we can parse out
4. **Transcript-parse failures: silent or loud?** Lean: silently drop transcript-derived fields, write the rest of the record, surface the issue in `/promptcellar doctor`. Agree? Good
5. `**.promptcellar/` private dir vs. `.prompts/` root.** I'm keeping them separate: `.prompts/` is the public PLF root (committed), `.promptcellar/` holds private state and per-repo config (the `state/` and `config.local.json` parts gitignored, `config.json` committed). Two new dirs in every repo. OK? Yes good
6. **Spec feedback to upstream.** PLF spec doesn't describe the recommended pattern for "captured prompt → outcome" lifecycle (i.e. how a tool gets from `UserPromptSubmit` to a complete record without violating §2.1). Worth a non-normative implementation-notes section. **Want me to draft that PR against** `promptcellar-format` **after you sign off here?**  I think we should figure out what the outcome is exactly in M0 and then indeed propose updates.
7. **Marketplace timing.** Once M0 confirms the marketplace listing requirements, do we list immediately or wait until after M3 when records are rich? Lean: list at M2 with M1-quality records and rely on plugin auto-update so users get on the official path early. Don't worry about marketplace yet. Let's get this thing working end-to-end and then we'll do those publication steps

