# Changelog

All notable changes to the Promptcellar plugin for Claude Code. The on-disk
[PLF format](https://github.com/dominiek/promptcellar-format) has its own
changelog.

## v0.7.0 — 2026-05-19

- **Captured prompts now record `cwd`** — the working directory the prompt
  was issued from, expressed as a path relative to the PLF store root. Fills
  the gap where `outcome.files_touched` paths (e.g. `app/sitemap.ts`,
  `cmd/pc-cli/main.go`) were ambiguous when a workspace destination pooled
  prompts from multiple sibling repos into one `.prompts/` store. Joining
  `<store-root>/<cwd>/<files_touched[i]>` now yields a resolvable path.
  Omitted from records when cwd == root (single-repo capture stays
  diff-clean) and from excluded stubs. Implements `plf-1`'s new optional
  `cwd` field — see [PLF spec §3.7](https://github.com/dominiek/promptcellar-format/blob/main/SPEC.md#37-cwd-optional).

## v0.6.0 — 2026-05-06

- **`claude plugin install` is now self-sufficient.** `plugin/bin/` ships
  shell-script shims that the hook manifest points at; on first SessionStart
  they download the matching release tarball into `plugin/bin/.real/` and
  exec the real Go binary. Previously the marketplace install reported
  success but every hook fired with `No such file or directory` because the
  binaries are gitignored and only the `curl | sh` installer fetched them.
  The shims are POSIX shell, fall back silently when the bootstrap can't
  run, and are bypassed by `install/install.sh` Phase 2 (which still does
  an eager fetch so curl users see download progress). Thanks
  [@andrewplummer](https://github.com/andrewplummer) (#14).
- **Manifest fields for the official marketplace** — `homepage`,
  `repository`, `license`, and `keywords` added to `plugin.json` ahead of
  submission to `claude-plugins-official`.
- **Docs** — README now documents both the `curl | sh` and
  `/plugin install` routes, with a Windows note pointing at the curl
  installer (the bootstrap shim is POSIX shell).

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
