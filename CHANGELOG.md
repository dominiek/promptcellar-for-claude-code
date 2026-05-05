# Changelog

All notable changes to the Promptcellar plugin for Claude Code. The on-disk
[PLF format](https://github.com/dominiek/promptcellar-format) has its own
changelog.

## v0.5.1 — 2026-05-05

- **Fix `/promptcellar:uninstall`** — the in-app uninstall used to edit only
  `installed_plugins.json` and `config.json`, leaving the cache directory
  under `~/.claude/plugins/cache/<marketplace>/promptcellar/` in place. Claude
  Code kept loading slash commands and hooks from there even after a restart.
  Uninstall now also removes the cache subtree (including the legacy
  `cache/local/promptcellar/` path) and drops the dedicated `promptcellar`
  marketplace from `known_marketplaces.json`. Captured `.prompts/` data is
  still left intact.

## v0.5.0 — 2026-05-05

- **Workspace destination** — run `claude` from a directory above multiple
  repos and point Promptcellar at a single capture target via
  `/promptcellar:destination <path>`. All sessions whose cwd is at or below
  the configured directory write to the same `.prompts/`, so cross-repo work
  stays unified instead of fragmenting per repo.
- **Installer fetches platform binaries** from the GitHub release matching
  the requested version, rather than requiring a local Go toolchain. Cuts
  first-install time on a clean machine to a few seconds.
- **Uninstall script** counterpart to `install.sh` for clean removal.

## v0.4.0 — 2026-04-30

- **Built-in secret detection.** Vendored the gitleaks default rule set
  (222 rules covering AWS, GCP, Azure, GitHub, Stripe, Anthropic, OpenAI,
  Slack, Twilio, etc.) plus a hand-rolled PII layer with Luhn-validated
  card numbers, ISO-13616 IBAN checks, and SSN/email/phone matchers. Always
  on, no configuration required.
- **`.promptcellarallow`** override file — same syntax as
  `.promptcellarignore`, inverse semantics. Narrows the built-in layers
  without weakening team-authored deny rules.
- **Path layout simplified** — dropped the hour bucket from `.prompts/`.
  Records now land at `.prompts/YYYY/MM/DD/<session-id>.jsonl` instead of
  `.prompts/YYYY/MM/DD/HH/<session-id>.jsonl`.
- Internal: removed the `promptcellar-format` submodule in favor of the
  vendored JSON Schema under `test/fixtures/`.

## v0.3.0 — 2026-04-29

Initial public release covering milestones M0–M5:

- **M0** — Discovery plugin reverse-engineered Claude Code's hook payloads
  and transcript shape.
- **M1** — Hook binaries (`SessionStart`, `UserPromptSubmit`, `PostToolUse`,
  `Stop`) with a buffer-then-flush state machine. Six integration scenarios
  green; sub-10ms cold start.
- **M2** — `curl | sh` installer, slash commands, three-layer
  enable/disable resolution (global / repo / personal),
  `.promptcellarignore`.
- **M3** — `PostToolUse` capture of `files_touched` and outcome status;
  transcript polling for token counts and cost.
- **M4** — CI integration tests, cross-build for darwin/linux/windows ×
  arm64/x64, release workflow.
- **M5** — MCP retrieval server (`promptcellar.{search,log,touched,session}`).
