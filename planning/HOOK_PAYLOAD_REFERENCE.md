# Hook Payload Reference — Claude Code 2.1.123

**Source:** M0 forensic dumps from `~/.promptcellar-discovery/eff7d519-…/` (88 hook fires across 6 prompts in one session, plus the live transcript JSONL inspected at multiple snapshot points).
**Targeted Claude Code version:** 2.1.123 (parsed from `CLAUDE_CODE_EXECPATH` and confirmed by transcript records' `version` field).
**Status:** Draft 1 (2026-04-30). This is the contract M1 production code is written against. Future CC versions get versioned adapters.

## 1. Hook surface (observed empirically)

Hooks that fired during the discovery session:

- `SessionStart`
- `UserPromptSubmit`
- `UserPromptExpansion` (only on slash-command invocations)
- `PreToolUse`
- `PostToolUse`
- `PostToolBatch`
- `Stop`

Hooks that did **not** fire:

- `SessionEnd` — does not exist in CC 2.1.123. Confirmed: capture pipeline must rely on next-`SessionStart` orphan recovery, never `SessionEnd`.

### 1.1 Common stdin fields (every hook event)

| Field              | Type   | Notes |
|--------------------|--------|-------|
| `cwd`              | string | The current working directory at hook fire. Equals `CLAUDE_PROJECT_DIR` in observed runs. |
| `hook_event_name`  | string | The event name (e.g. `"UserPromptSubmit"`). Useful for shared dispatchers. |
| `permission_mode`  | string | `"default"` or `"acceptEdits"` observed. Tells us when the user is in plan-mode / etc. |
| `session_id`       | string | UUID. Stable across the entire CC session. |
| `transcript_path`  | string | Absolute path to the live session transcript JSONL (see §3). |

### 1.2 Common environment variables

| Var                    | Notes |
|------------------------|-------|
| `CLAUDE_CODE_ENTRYPOINT` | `"cli"` observed. |
| `AI_AGENT`             | e.g. `"claude-code/2.1.123/harness"` — **`tool.version` is the second slash-separated component**. Reliably set in every hook subprocess on CC 2.1.x. |
| `CLAUDE_CODE_EXECPATH` | NOT set in hook subprocesses (corrected M1 — earlier draft incorrectly listed it; that env var leaks from the user's *interactive* shell, not from CC's hook fork). Kept as a defensive fallback if future CC versions start exposing it. |
| `CLAUDE_PLUGIN_ROOT`   | Absolute path to the plugin's install dir. Use this in plugin-relative paths (it's set even for `--plugin-dir` dev installs — confirmed). |
| `CLAUDE_PLUGIN_DATA`   | A per-plugin scratch dir under `~/.claude/plugins/data/`. |
| `CLAUDE_PROJECT_DIR`   | The cwd at session launch (not always a git root — non-git dirs were tested). |
| `CLAUDE_ENV_FILE`      | **`SessionStart` only** — a path the hook can write `KEY=VALUE` lines into to persist env vars for the rest of the session. |

## 2. Per-event payload schema

### 2.1 `SessionStart`

Additional fields beyond §1.1:

- `model`: e.g. `"claude-opus-4-7[1m]"` — full user-visible model identity (with context-tier suffix).
- `source`: e.g. `"startup"`. Other values (resume, etc.) not observed.

**Notable:** the `model` field is **only** in `SessionStart`. It is not provided at `UserPromptSubmit` or `Stop`. Promptcellar caches it on `SessionStart`.

### 2.2 `UserPromptSubmit`

- `prompt`: verbatim text the user typed. Slash-command text is preserved as-is (e.g. `"/loop"` arrives as the exact string `"/loop"`).
- No model field, no token info, no transcript-summary info — only the raw prompt.

### 2.3 `UserPromptExpansion`

Fires when the user invokes a slash command, **in addition to** `UserPromptSubmit`, ~30ms apart, same prompt text in both.

- `prompt`: `"/foo"` (verbatim)
- `command_name`: `"foo"`
- `command_args`: e.g. `""` or whatever followed the command name
- `command_source`: e.g. `"bundled"`. (Other observed values not yet captured — likely `"plugin"`, `"user"` for user-defined commands.)
- `expansion_type`: `"slash_command"` observed.

**Implication for Promptcellar:** the `UserPromptSubmit` already carries the verbatim prompt text the user typed (`/foo`). `UserPromptExpansion` is informational only. We capture from `UserPromptSubmit` and ignore `UserPromptExpansion` for v1. (We may later record `command_name` as an `x_claude_code` extension field if useful for analytics.)

### 2.4 `PreToolUse`

- `tool_name`: e.g. `"Bash"`, `"Read"`, `"Edit"`, `"Write"`.
- `tool_input`: tool-specific structured args. For `Bash`: `{ "command": "...", "description": "..." }`. For `Edit`: `{ "file_path", "old_string", "new_string", "replace_all" }`. Etc.
- `tool_use_id`: e.g. `"toolu_01TeU5v7DpdVBsc5JyxzstWh"` — Anthropic-issued ID; pairs 1:1 with `PostToolUse`.

### 2.5 `PostToolUse`

All `PreToolUse` fields plus:

- `tool_response`: structured for some tools, opaque-string for others. For `Bash`: `{ "interrupted", "isImage", "noOutputExpected", "stderr", "stdout" }`. For `Read`/`Edit`: a result string.
- `duration_ms`: per-tool wall-clock duration.

### 2.6 `PostToolBatch`

Fires once per "batch of completed tool calls" — appears to fire after each agent turn that ran tools.

- `tool_calls`: array. Each element has `tool_name`, `tool_input`, `tool_response` (here always a **string**, even when `PostToolUse` had a structured response), `tool_use_id`.
- No `duration_ms` at the batch level.

**Implication:** `PostToolBatch` is a slightly redundant aggregate over the individual `PostToolUse` fires of the same turn. For Promptcellar, `PostToolUse` is sufficient and arrives sooner. We can ignore `PostToolBatch` unless we need a reliable "turn complete" signal.

### 2.7 `Stop`

- `last_assistant_message`: the full assistant response as a string. **This is `outcome.summary` for free** — no transcript parsing required. Just truncate to PLF's ≤500-char recommendation.
- `stop_hook_active`: boolean. `false` for normal session stops; presumably `true` if the hook itself triggers another Stop (recursion guard).

**Notable absence:** the assistant's token usage is **not** in the `Stop` payload. See §3 / §4.

## 3. Transcript schema

Lives at `~/.claude/projects/<dash-encoded-cwd>/<session-id>.jsonl` (e.g. `/Users/dodo/checkouts/foo` → `-Users-dodo-checkouts-foo`). The exact path is provided to every hook as `transcript_path` — never hardcode.

### 3.1 Record types observed

From a 114-line full-session transcript:

| `type`                  | count | purpose |
|-------------------------|-------|---------|
| `attachment`            | 43    | system-injected sub-records: `hook_success`, `deferred_tools_delta`, `mcp_instructions_delta`, `skill_listing` |
| `user`                  | 21    | user prompts AND tool results returning to the agent |
| `assistant`             | 19    | agent turns (text/thinking/tool_use), with model + token usage |
| `file-history-snapshot` | 10    | file-edit tracking |
| `system`                | 10    | system messages |
| `last-prompt`           | 3     | bookmark of last user prompt (UUID pointer) |
| `permission-mode`       | 3     | permission-mode setting markers |
| `ai-title`              | 3     | auto-generated session title (`{type:"ai-title", aiTitle:"…"}`) |
| `queue-operation`       | 2     | internal; not relevant to capture |

### 3.2 Common fields (most record types)

`parentUuid`, `uuid`, `sessionId`, `timestamp` (RFC 3339, ms precision), `userType:"external"`, `entrypoint:"cli"`, `cwd`, **`version`** (CC version, e.g. `"2.1.123"`), **`gitBranch`** (e.g. `"main"`).

So: `tool.version` and `git.branch` can both be sourced from any single transcript line, no shell-out needed.

### 3.3 `type:"user"` shape

```json
{
  "type": "user",
  "uuid": "...", "parentUuid": "...",
  "promptId": "<uuid>",
  "message": { "role": "user", "content": "what time is it" },
  "timestamp": "...", "permissionMode": "default",
  "userType": "external", "entrypoint": "cli",
  "cwd": "...", "sessionId": "...", "version": "2.1.123", "gitBranch": "main"
}
```

`promptId` is an Anthropic-issued UUID for the prompt — distinct from our `record.id` but worth preserving as `x_claude_code.prompt_id` for cross-reference.

### 3.4 `type:"assistant"` shape (the gold)

```json
{
  "type": "assistant",
  "uuid": "...", "parentUuid": "...",
  "requestId": "req_...",
  "message": {
    "model": "claude-opus-4-7",
    "id": "msg_...",
    "role": "assistant",
    "content": [
      {"type": "text", "text": "..."},
      {"type": "thinking", "thinking": "...", "signature": "..."},
      {"type": "tool_use", "id": "toolu_...", "name": "Bash", "input": {...}, "caller": {"type": "direct"}}
    ],
    "stop_reason": "end_turn" | "tool_use",
    "usage": {
      "input_tokens": 6,
      "output_tokens": 40,
      "cache_creation_input_tokens": 10378,
      "cache_read_input_tokens": 14865,
      "service_tier": "standard",
      "speed": "standard",
      "iterations": [...]
    }
  },
  "timestamp": "...", "version": "2.1.123", "gitBranch": "main", ...
}
```

**Token counts live in `message.usage`.** Field names map cleanly to PLF:

| PLF field                          | Transcript path                              |
|------------------------------------|----------------------------------------------|
| `enrichments.tokens.input`         | `message.usage.input_tokens`                 |
| `enrichments.tokens.output`        | `message.usage.output_tokens`                |
| `enrichments.tokens.cache_read`    | `message.usage.cache_read_input_tokens`      |
| `enrichments.tokens.cache_write`   | `message.usage.cache_creation_input_tokens`  |

Note: `message.model` here is the canonical short form (`claude-opus-4-7`), without the `[1m]` context-tier suffix that `SessionStart`'s payload had. For PLF `model.name`, prefer the SessionStart value (more informative).

### 3.5 Slicing tokens to a single prompt

The transcript is a flat append-only log of all turns in the session. To attribute tokens to a specific prompt:

1. Find the `type:"user"` record whose `promptId` matches the prompt we're flushing.
2. Walk forward, collecting all `type:"assistant"` records until the next `type:"user"` record (or EOF).
3. Sum their `message.usage.{input,output,cache_*}_tokens`.

Each prompt may produce multiple assistant records (text turn + tool-use turn + post-tool text turn etc.) — sum them all.

## 4. Critical timing constraint

**At `Stop` hook time, the just-completed assistant record is NOT yet in the transcript file. By the very next hook fire, it is.**

Empirically (counts of `type:"assistant"` records in the transcript snapshot at each hook fire):

| seq  | hook event           | transcript lines | user records | assistant records |
|------|----------------------|-----------------:|-------------:|------------------:|
| 0003 | `Stop` (prompt #1)   |                9 |            1 |             **0** |
| 0004 | `UserPromptSubmit`   |               13 |            1 |             **1** |
| 0005 | `PreToolUse`         |               15 |            2 |                 1 |
| 0026 | `Stop` (prompt #2)   |               64 |            9 |                12 |
| 0027 | `UserPromptSubmit`   |               68 |            9 |                13 |

So between `Stop` and the next hook fire of any kind, Claude Code persists the assistant record. The "next hook" is reliably enough delay; we do not need a timer or polling.

### 4.1 Implication for the capture pipeline

Original plan: flush full PLF record at `Stop`. **This must change.**

Revised:

| Event                | State action |
|----------------------|--------------|
| `UserPromptSubmit`   | Mint id; buffer prompt + git snapshot to `.promptcellar/state/<session>.json`. |
| `PostToolUse`        | Append `tool_input.file_path` (or equivalent) to buffered `files_touched`. |
| `Stop`               | Set `stop_seen=true`, `last_assistant_message`, `duration_ms` in state. **Do not flush.** |
| **Next hook (any)**  | If buffered prompt has `stop_seen=true`: re-read transcript, locate assistant records for this `promptId`, compute `enrichments.tokens.*`, flush full PLF record to `.prompts/...jsonl`, clear state. |
| `UserPromptSubmit` (when prior prompt's `stop_seen=false`) | Flush prior prompt as `outcome.status="interrupted"`, no `enrichments`. |

The "next hook" can be the next `UserPromptSubmit` (typical case) or a `PreToolUse` from a follow-up turn; doesn't matter — any subsequent hook fire is enough time for the transcript to flush.

**Edge case — last prompt of a session:** if the user never submits another prompt and never starts another tool, the last prompt's enrichments stay missing until the *next session* in this repo, where `SessionStart`'s orphan-recovery sweep finalizes it (the transcript by then has all records persisted). The `outcome.summary` is fine because `last_assistant_message` was captured at Stop. Only `enrichments.tokens.*` would be missing in the meantime.

## 5. Slash-command flow

User typed `/loop`:

1. `t=0ms`     — `UserPromptExpansion` fires. Payload: `prompt:"/loop"`, `command_name:"loop"`, `expansion_type:"slash_command"`.
2. `t=27ms`    — `UserPromptSubmit` fires. Payload: `prompt:"/loop"`.
3. Agent processes the slash-command-resolved prompt.
4. `Stop` fires.

For Promptcellar v1: capture only on `UserPromptSubmit`. The verbatim text `/loop` is preserved. `UserPromptExpansion` is informational and ignored.

## 6. Interrupted / preempted prompts

Observed in the dumps: prompt `0040` (`UserPromptSubmit`) had no `Stop` event before prompt `0041` arrived 34s later. The user submitted a new prompt while the agent was still working (or before its `Stop` had fired).

Implication: a fresh `UserPromptSubmit` arriving while a previous prompt's state has `stop_seen=false` indicates a preemption. Promptcellar flushes the prior prompt with `outcome.status="interrupted"` and whatever partial data we have, then buffers the new prompt as a fresh record.

## 7. Promptcellar field map (final)

| PLF field                          | Source (verified in dumps)                                                             |
|------------------------------------|-----------------------------------------------------------------------------------------|
| `version`, `id`, `timestamp`       | Generated locally                                                                       |
| `session_id`                       | Any hook payload                                                                        |
| `prompt`                           | `UserPromptSubmit.stdin.prompt`                                                         |
| `author.email/name/id`             | `git config user.email`/`user.name`/`user.signingkey`                                    |
| `tool.name`                        | Hardcoded `"claude-code"`                                                               |
| `tool.version`                     | Trailing `<X.Y.Z>` component of `CLAUDE_CODE_EXECPATH` (or transcript `version`)        |
| `model.provider`                   | Hardcoded `"anthropic"`                                                                 |
| `model.name`                       | `SessionStart.stdin.model` (e.g. `"claude-opus-4-7[1m]"`); cached for the session       |
| `model.version`                    | null (no separate version)                                                              |
| `git.branch`                       | Transcript `gitBranch` (cheaper than shell-out; available on every transcript line)     |
| `git.head_commit`                  | `git rev-parse HEAD` (not in transcript)                                                |
| `git.dirty`                        | `git status --porcelain` (not in transcript)                                            |
| `parent.prompt_id`                 | Tracked in session state                                                                |
| `outcome.summary`                  | `Stop.stdin.last_assistant_message` truncated to ≤500 chars                             |
| `outcome.files_touched`            | `PostToolUse.stdin.tool_input.file_path` for `Edit`/`Write`/`MultiEdit`/`NotebookEdit`   |
| `outcome.commits`                  | `git log --since=<prompt_ts>` at flush time                                             |
| `outcome.status`                   | Stop seen → `"completed"`; preempted → `"interrupted"`; tool-error pattern → `"errored"` |
| `enrichments.duration_ms`          | `Stop.wall_ts` − `UserPromptSubmit.wall_ts`                                             |
| `enrichments.tokens.input`         | Σ assistant records' `message.usage.input_tokens` between this prompt's `type:"user"` and the next |
| `enrichments.tokens.output`        | Σ `message.usage.output_tokens`                                                         |
| `enrichments.tokens.cache_read`    | Σ `message.usage.cache_read_input_tokens`                                               |
| `enrichments.tokens.cache_write`   | Σ `message.usage.cache_creation_input_tokens`                                           |
| `enrichments.cost_usd`             | Computed from tokens × Anthropic published prices (table bundled, refresh on update)    |

`x_claude_code` extension fields worth capturing for cross-reference (PLF spec §3.11 reserves `x_<tool>` for tool-specific extensions):

- `x_claude_code.prompt_id` ← transcript `user` record `promptId`
- `x_claude_code.request_id` ← assistant record `requestId` (for billing reconciliation)
- `x_claude_code.command_name` ← `UserPromptExpansion.command_name` when the prompt was a slash command
- `x_claude_code.permission_mode` ← `UserPromptSubmit.permission_mode`

## 8. v2 harvest findings (2026-04-30, six additional sessions)

### 8.1 Headless / SDK-CLI mode (session `ec576183`)

`claude --plugin-dir … -p "what time is it"` confirmed:

- All hooks fire normally: `SessionStart` → `UserPromptSubmit` → `Stop`.
- `CLAUDE_CODE_ENTRYPOINT = "sdk-cli"` (vs `"cli"` for interactive) — this is how we detect headless mode.
- **`SessionStart.stdin.model` is ABSENT** in headless mode. The interactive sessions have `model: "claude-opus-4-7[1m]"`; headless has no `model` field at all.
- `Stop.last_assistant_message` is fully populated, same as interactive.
- Transcript at Stop has 7 lines and 0 assistant records (same async-persistence pattern; no headless-specific behavior).

**Implication for `model.name`:** the SessionStart-only path doesn't work in headless. Fallback chain:
1. `SessionStart.stdin.model` if present.
2. Else, at flush time, read the most recent `type:"assistant"` record from the transcript and use `message.model`.
3. Else (no assistant records yet) write `model.name = "claude-opus-4-7"` as a hardcoded default for plf-1 — or hold the record until enrichments are computable. Prefer #2.

### 8.2 Session resume (session `e1d4ef61`)

User invoked `/resume` (or equivalent) mid-session. Result:

- `SessionStart` fired AGAIN within the **same `session_id`**. The dumps show two SessionStarts (`0001` with `source:"startup"`, `0007` with `source:"resume"`).
- The resume `SessionStart` payload **lacks `model`** — same as headless mode. The `model` field appears only on the first (`startup`) SessionStart.
- `transcript_stats` on the resume SessionStart shows the existing transcript: `assistant: 1` confirms prior activity is preserved.

**Implication for the capture pipeline:** orphan-recovery at `SessionStart` should filter on **other** session_ids — never re-flush state belonging to the *current* session_id. Idempotent-by-construction. This is what we already designed; no change needed.

### 8.3 Plugin-shipped slash commands (session `e1d4ef61`, expansion at seq 2)

User invoked `/discovery-ping`. The `UserPromptExpansion` payload:

- `command_name: "promptcellar-discovery:discovery-ping"` — **plugin commands are auto-namespaced as `<plugin-name>:<command-name>`**.
- `command_source: "plugin"` (vs `"bundled"` for built-ins like `/loop`).
- `prompt: "/promptcellar-discovery:discovery-ping"` — the verbatim prompt sent to `UserPromptSubmit` is the namespaced form, not the bare `/discovery-ping` the user typed.

**Implication for Promptcellar's commands:** if we name the plugin `promptcellar`, our commands appear as `/promptcellar:status`, `/promptcellar:enable`, etc. Users can usually type the bare form and CC resolves the namespace, but the recorded `prompt` text is the namespaced form. The `IMPLEMENTATION_PLAN.md` slash-command surface should reflect this.

### 8.4 Tool errors and the `PostToolUse` gap (session `d04efd9c`, prompt 1)

Prompt: "Please cat GOBLINS.md" (file does not exist). Hook sequence:

- `0003-PreToolUse` (`Read`, `file_path=".../GOBLINS.md"`)
- `0004-PostToolBatch` (`tool_response: "File does not exist. Note: …"`)
- `0005-Stop`
- **No `PostToolUse` fired** for the failed `Read`.

This is a real divergence: when a tool errors out without producing a normal result, CC may skip `PostToolUse` and only fire `PostToolBatch`.

**Implications:**

- For `outcome.files_touched`: source from `PostToolUse` only — failed Edits/Writes are not recorded, which is the correct semantics (they didn't actually touch the file).
- For `outcome.status = "errored"`: cannot rely on `PostToolUse` signal alone. Scan `PostToolBatch.tool_calls[].tool_response` for error markers (e.g., starts with "File does not exist", "Error:", non-zero exit, etc.) — or treat any tool missing a `PostToolUse` follow-up as suspect.
- `PostToolBatch` is the more reliable "tool completed" signal. It fires once per agent turn, aggregating all `tool_use_id`s, and includes responses for both successful and failed tools.

We previously planned to ignore `PostToolBatch`. **Revised:** use `PostToolUse` for `files_touched` (correctness), use `PostToolBatch` for `outcome.status` error detection.

### 8.5 Multi-tool turn (session `d04efd9c`, prompt 2)

Prompt: "Please cat CHANGELOG.md, README.md and SPEC.md in one go". Sequence:

- 2 `PreToolUse` / 2 `PostToolUse` pairs (only 2, not 3 — likely batched or one was cached).
- 1 `PostToolBatch` aggregating both.
- 1 `Stop`.

Confirms: `PostToolBatch` fires once per turn regardless of how many tool calls within. Nothing surprising.

### 8.6 Non-git directory (session `3bd4ef01`)

User opened CC in `/private/tmp/pc-discovery-nongit` (no `.git/`). Result:

- `SessionStart` fired. `cwd` and `CLAUDE_PROJECT_DIR` both resolve to the non-git path.
- `model` is present (`claude-opus-4-7[1m]`).
- `transcript_path` points to the projects dir (`-private-tmp-pc-discovery-nongit/…`), but the file did not exist at SessionStart time (no `transcript_stats`).
- User did not submit a prompt, so we don't have data on what `gitBranch` looks like in non-git transcripts.

**Implication:** doesn't change the design — Promptcellar already treats non-git directories as out of scope. The non-git SessionStart will fire, the resolve-config check finds no `.git/`, and capture is skipped. Safe.

### 8.7 Concurrent sessions (sessions `aa7f876e` and `e93b647e`)

Two terminal tabs, same repo. Confirmed:

- Distinct `session_id`s → distinct dump dirs (`aa7f876e-…/` and `e93b647e-…/`).
- Each session writes its own transcript file under `~/.claude/projects/-Users-dodo-checkouts-promptcellar-format/<session-id>.jsonl`. No transcript-file conflict.
- The same applies to `.prompts/<YYYY>/<MM>/<DD>/<HH>/<session-id>.jsonl` outputs in production — one writer per file, by construction.

Validates the merge-conflict-free property of PLF.

### 8.8 Still uncovered in M0

- **Ctrl-C interrupt as distinct from preemption** — exercised but indistinguishable from preemption in the dumps (both leave state files with `stop_seen=false`). For our purposes, both are `outcome.status="interrupted"` — no need to distinguish.
- **Hook ordering** when multiple hook-using plugins register the same event — will revisit when Promptcellar coexists with hookify or similar.
- **Windows behavior** — discovery ran on macOS only. Need a Windows pass before we sign off Windows support; deferred to a later milestone.

## 9. Plan changes triggered by these findings

Apply to `IMPLEMENTATION_PLAN.md`:

1. **§1 capture pipeline** — change "Stop → flush" to "Stop → mark stop_seen; flush at next hook fire". Update the table accordingly.
2. **§4 metadata table** — `git.branch` source can be transcript (cheaper); `tool.version` confirmed via `CLAUDE_CODE_EXECPATH`; `outcome.summary` confirmed via `Stop.last_assistant_message` (no transcript parse needed); add `model.name` source as `SessionStart.stdin.model`.
3. **§5.1** — mark M0 deliverable complete. Reference this document.
4. **§7 repo layout** — keep `discovery-plugin/` (this is a useful artifact for regression testing).
5. **§11 risks** — add: "Last-prompt-of-session enrichments are deferred until the next session's SessionStart. Acceptable; alternative would require a daemon."
