# Promptcellar Logging Format — `plf-1`

`plf-1` is the on-disk format Promptcellar uses to capture agentic coding prompts. It is an open standard, intended to be writable by any coding agent (Claude Code, Cursor, Aider, Codex, etc.) and readable by any tool. The version string is included in every record so future versions can coexist on disk.

## Goals

- Capture the human signal — every prompt a developer types to an agent.
- Merge-conflict-free across branches by construction.
- Tool- and model-agnostic.
- Append-only and chronological.

## Directory layout

```
.prompts/
  YYYY/
    MM/
      DD/
        <session-id>.jsonl
```

- Bucketed by **session start date** in UTC.
- One JSONL file per session. Each line is one prompt record.
- `<session-id>` is a UUIDv4 (or any globally-unique string the agent picks).
- Files are append-only. A session that crosses a day boundary keeps writing to the file in its start-day bucket.

### Why this avoids merge conflicts

A session belongs to one agent process on one machine. Two branches cannot write to the same session file because they generate different session IDs. Different sessions live in different files. Git only ever sees adds.

## Record schema

Each line is one JSON object — one prompt, one record.

### Required fields

| Field | Type | Notes |
|---|---|---|
| `version` | string | Always `"plf-1"`. |
| `id` | string | UUID for this record. |
| `session_id` | string | Matches the filename. Stable across all prompts in the session. |
| `timestamp` | string | RFC 3339 UTC. |
| `author` | object | See below. |
| `tool` | object | See below. |
| `model` | object | See below. |
| `prompt` | string | Raw prompt text the user typed. |

### `author`

| Field | Type | Notes |
|---|---|---|
| `email` | string | Git `user.email`. |
| `name` | string | Git `user.name`. |
| `id` | string \| null | Optional stable ID (signing-key fingerprint, SSO subject). |

### `tool`

| Field | Type | Notes |
|---|---|---|
| `name` | string | e.g. `"claude-code"`, `"cursor"`, `"aider"`. |
| `version` | string | Tool version. |

### `model`

| Field | Type | Notes |
|---|---|---|
| `provider` | string | e.g. `"anthropic"`, `"openai"`, `"local"`. |
| `name` | string | e.g. `"claude-opus-4-7"`. |
| `version` | string \| null | Specific version pin if known. |

### Optional: `git`

| Field | Type |
|---|---|
| `branch` | string |
| `head_commit` | string (SHA) |
| `dirty` | boolean |

### Optional: `parent`

| Field | Type | Notes |
|---|---|---|
| `prompt_id` | string | Prior prompt in the same session this one follows from. Usually the immediately preceding record; explicit so reorderings stay representable. |

### Optional: `outcome`

A **summary** of what the agent did in response. Deliberately not a full transcript — the audit unit is the summary, not every tool call.

| Field | Type | Notes |
|---|---|---|
| `summary` | string | Short natural-language summary (≤ ~500 chars). |
| `files_touched` | array of strings | Repo-relative paths the agent edited. |
| `commits` | array of strings | Commit SHAs the prompt contributed to. |
| `status` | string | `"completed"`, `"errored"`, `"interrupted"`, `"unknown"`. |

### Optional: `enrichments`

Cost and time attribution. All optional — recorded if the agent has them.

| Field | Type |
|---|---|
| `tokens.input` | integer |
| `tokens.output` | integer |
| `tokens.cache_read` | integer |
| `tokens.cache_write` | integer |
| `cost_usd` | number |
| `duration_ms` | integer |

### Optional: `excluded`

When a prompt matches `.promptcellarignore`, a stub record is written instead of dropping silently, so the timeline is not gappy:

```json
{
  "version": "plf-1",
  "id": "…",
  "session_id": "…",
  "timestamp": "…",
  "author": { … },
  "tool": { … },
  "model": { … },
  "excluded": { "reason": "matched .promptcellarignore", "pattern_id": "secrets" }
}
```

When `excluded` is present, `prompt`, `outcome`, `enrichments`, `git`, and `parent` are omitted.

## Example record

```json
{"version":"plf-1","id":"3f1c…","session_id":"a91d…","timestamp":"2026-04-27T14:32:08.142Z","author":{"email":"dominiek@rekall.ai","name":"Dominiek"},"tool":{"name":"claude-code","version":"2.4.0"},"model":{"provider":"anthropic","name":"claude-opus-4-7","version":null},"prompt":"refactor the auth middleware to use the new session API","git":{"branch":"feat/auth-rewrite","head_commit":"a1b2c3d","dirty":true},"outcome":{"summary":"Replaced cookie middleware with token-based equivalent across server/auth/. Added 4 tests.","files_touched":["server/auth/middleware.ts","server/auth/middleware.test.ts"],"commits":["e4f5g6h"],"status":"completed"},"enrichments":{"tokens":{"input":12400,"output":2810},"cost_usd":0.18,"duration_ms":48210}}
```

## `.promptcellarignore`

A repo-root file listing patterns whose match against the **prompt text** causes the prompt to be excluded from capture. Same spirit as `.gitignore`, but the matching target is the prompt string, not file paths.

### Format

- One pattern per line. Comments start with `#`. Blank lines ignored.
- Patterns are POSIX extended regular expressions, case-insensitive by default.
- A line `id: <name>` immediately above a pattern names it. The name is recorded as `excluded.pattern_id`. Optional but recommended.
- The file is committed to the repo so the team shares one definition.

### Behavior

- If any pattern matches the prompt text, the entire prompt is excluded and a stub record (see `excluded`) is written in its place.
- Matching happens locally **before** any prompt is written to disk. There is no captured-then-redacted intermediate state.

### Example

```
id: secrets
(AWS_SECRET_ACCESS_KEY|GITHUB_TOKEN|OPENAI_API_KEY)

id: security-paths
\bsecurity/(runbooks|incident)\b

id: credential-shapes
(ghp_[A-Za-z0-9]{36}|sk-[A-Za-z0-9]{32,})
```

## Versioning

- `version` is the contract. `plf-1` records remain readable by all future tools.
- Additive changes (new optional fields) do not bump the version.
- Breaking changes bump to `plf-2`. Both may coexist in one repo.

## Out of scope

- Full transcripts and tool-call traces — the audit unit is `outcome.summary`. Tools that want richer traces can store them separately and link by `id`.
- Indexing and search — a tooling layer above `plf-1`, not part of the format.
- Encryption — Promptcellar's premise is shared visibility into the human signal that built the code; teams who can't commit prompts plaintext shouldn't adopt the format.
