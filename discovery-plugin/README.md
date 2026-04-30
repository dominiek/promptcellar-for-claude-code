# promptcellar-discovery — M0 forensic-dump plugin

A throwaway Claude Code plugin that registers every hook entry point and dumps the full hook payload plus a transcript snapshot to a per-session forensic log. Used to pin down the actual extensibility surface of Claude Code so the Promptcellar v1 design rests on observed behavior rather than assumed contracts.

**Not for production use.** This plugin records everything Claude Code passes to its hooks, including the full process environment. That's the point — so we can read it later. Don't run it on a session you'd be uncomfortable archiving locally.

## What it captures

For every hook fire, writes to `~/.promptcellar-discovery/<session-id>/`:

- `<NNNN>-<EventName>.json` — sequence number, event, wall timestamp, full parsed stdin payload, **full environment** (`env_full`), argv, cwd, pid, and `transcript_stats` (line count + record-type counts of the transcript at this hook fire).
- `<NNNN>-<EventName>.transcript.jsonl` — copy of the file at `transcript_path` at that exact moment.

Hooks registered: `SessionStart`, `UserPromptSubmit`, `UserPromptExpansion`, `PreToolUse`, `PostToolUse`, `PostToolBatch`, `Stop`. (Claude Code does not provide a `SessionEnd` hook in 2.1.x — confirmed via M0 first harvest.)

Slash command shipped: `/discovery-ping` — replies "pong". Exists only to exercise the plugin-shipped slash command flow so we can observe `UserPromptExpansion.command_source` for non-bundled commands.

## Install (development mode)

```sh
cd /Users/dodo/checkouts/promptcellar-for-claude-code
claude --plugin-dir ./discovery-plugin
```

Each `claude --plugin-dir` invocation enables the plugin for that one session. Pass it again to reinvoke.

## v1 harvest (done)

The first harvest (session `eff7d519-…`) covered: normal prompt, file edit, Bash tool call, secret-shaped prompt, slash-command invocation, multi-prompt session. Findings folded into `planning/HOOK_PAYLOAD_REFERENCE.md`.

## v2 harvest (this round) — gap-coverage scenarios

Run `claude --plugin-dir ./discovery-plugin` and exercise the scenarios below. Each scenario produces its own `~/.promptcellar-discovery/<session-id>/` directory, so you can run them independently.

| # | Scenario | What we want to learn |
|---|----------|------------------------|
| 1 | **Plugin slash command** — invoke `/discovery-ping` from inside Claude Code. | `UserPromptExpansion.command_source` value when the command comes from a plugin (vs `"bundled"` for built-ins). Verifies plugin commands actually appear in the picker. |
| 2 | **Ctrl-C interrupt** — submit a long prompt (e.g. "list every file recursively under /Users/dodo and describe each"), hit Ctrl-C while the agent is generating. | Whether `Stop` fires on user-interrupt, with what `last_assistant_message`. Distinguishes Ctrl-C from preemption. |
| 3 | **Session resume** — exit the CC session (Ctrl-D twice or `/exit`), then re-run `claude --plugin-dir ./discovery-plugin --resume` (or however resume is invoked). | `SessionStart.stdin.source` value for resumed sessions. Confirms whether resume gets a new `session_id` or the old one. |
| 4 | **Headless mode** — `claude --plugin-dir ./discovery-plugin -p "what time is it"`. | Whether hooks fire in print/headless mode, and which ones. Critical for our Layer-2 automated test runner (M4). |
| 5 | **Non-git directory** — `mkdir /tmp/pc-discovery-nongit && cd /tmp/pc-discovery-nongit && claude --plugin-dir ~/checkouts/promptcellar-for-claude-code/discovery-plugin`, submit any prompt. | What `gitBranch` looks like in transcript records when no git is present. What `cwd`/`CLAUDE_PROJECT_DIR` resolve to. Whether CC even allows running there. |
| 6 | **Concurrent sessions** — open two terminal tabs, both running `claude --plugin-dir ./discovery-plugin` in the same repo. Submit a prompt in each. | Two distinct session-ids in `~/.promptcellar-discovery/`. Validates merge-conflict-free property of PLF (one session = one file, no overlap). |
| 7 | **Tool error** — ask Claude to run `cat /nonexistent-file-1234`. | `PostToolUse.tool_response` shape on error. Helps us classify `outcome.status="errored"`. |
| 8 | **Multi-tool turn** — ask "read the README files in 3 sibling directories of this repo". The agent should batch multiple Read calls in one turn. | `PostToolBatch.tool_calls` aggregation. Confirms whether batch fires once per turn or once per tool call. |

You don't need to run all eight in one go — even just 1, 2, 4, and 5 give us most of the remaining signal.

## Inspect the dumps

```sh
ls ~/.promptcellar-discovery/
ls ~/.promptcellar-discovery/<session-id>/
```

Each dump JSON includes `transcript_stats` showing `line_count` and `type_counts` per hook fire — so timing analysis ("when did the assistant record appear?") is just `jq '.transcript_stats.type_counts.assistant'` across the dumps.

## Output

After the v2 harvest, I'll update `planning/HOOK_PAYLOAD_REFERENCE.md` with the new findings (mark each scenario answered, fold any surprises into the relevant section), then we lock M1 and start writing the production hooks.

## Cleanup

```sh
rm -rf discovery-plugin/                # remove the plugin source
rm -rf ~/.promptcellar-discovery/       # remove the dumps
```

No state outside those two locations.
