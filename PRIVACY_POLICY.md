# Privacy Policy

_Last updated: 2026-05-06_

Promptcellar for Claude Code (the "plugin") is a local Claude Code plugin that captures the prompts you submit to Claude Code and writes them to disk in a location you choose. This policy explains what the plugin does — and does not — do with that data.

## You control your data

**The plugin does not transmit your prompts, code, or any captured data to Promptcellar, the plugin authors, or any third party.** Everything captured stays on your machine, inside the directory you configure (typically `.prompts/` in the repository where you ran Claude Code).

You decide:

- **Where prompts are stored.** By default, the plugin writes to `.prompts/` at your repo root. You can redirect capture to any directory you control via `/promptcellar:destination`, or disable capture entirely with `/promptcellar:disable`.
- **Whether prompts are committed.** `.prompts/` is a normal directory. Commit it, gitignore it, encrypt it, sync it to a private repo — it is yours to manage like any other file in your project.
- **Who can see it.** Because the data lives on your filesystem (and only there), access is governed by your filesystem permissions, your git remote's access controls, and any other tooling you choose to apply.
- **When to delete it.** Removing captured data is `rm` on the files. There is no remote copy for us to retain or expire.

## What the plugin captures

For each prompt you submit, the plugin writes one JSONL record under your configured destination containing:

- The prompt text.
- Your local git author identity (name and email, as configured in git).
- The model and tool versions in use.
- A snapshot of git state at prompt time (branch, HEAD, dirty/clean).
- A summary of what the agent did in response (files touched, commits created, status).
- Token usage, estimated cost, and wall-clock duration.

Records are append-only JSONL files. The full on-disk format is defined by the open [Promptcellar Format (PLF)](https://github.com/dominiek/promptcellar-format) specification.

## Built-in redaction

Before any prompt is written to disk, the plugin runs it through a built-in log-redaction matcher. Prompts that match a known secret or PII pattern (API keys, credit-card numbers, IBANs, US SSNs, emails, phone numbers, and 200+ vendor token shapes from the gitleaks catalog) are replaced with an `excluded` stub — the prompt text is not stored. Teams can extend this with `.promptcellarignore` (additional deny patterns) and narrow it with `.promptcellarallow`. See the [README](./README.md#log-redaction) for details.

Redaction runs locally and is best-effort. It reduces the chance that secrets land in `.prompts/`, but it is not a substitute for your own review of what you commit.

## Network activity

The plugin itself performs no network requests during normal capture. The only network activity initiated by this project is:

- **First-run bootstrap.** On first use, the plugin downloads the matching platform binaries from the project's GitHub Releases page. This is a standard GitHub HTTPS request and is subject to GitHub's privacy practices.
- **Installer download.** The `curl | sh` installer fetches a shell script from `get.promptcellar.io` and binaries from GitHub Releases.

No telemetry, analytics, error reporting, or "phone home" requests are made by the plugin. We do not run a backend that receives data from your installation.

## Third parties

The plugin runs as part of Claude Code, which is operated by Anthropic and has its own privacy policy governing your use of Claude. This plugin's policy covers only the local capture-and-storage layer added by the plugin.

If you choose to commit `.prompts/` to a git host (GitHub, GitLab, etc.) or sync it to another service, that host's privacy policy applies to the copy stored there. The plugin does not push to any host on your behalf.

## Open source

The plugin is MIT-licensed and the source is public at [dominiek/promptcellar-for-claude-code](https://github.com/dominiek/promptcellar-for-claude-code). You can audit exactly what is captured and where it goes.

## Contact

Questions or concerns: open an issue at [dominiek/promptcellar-for-claude-code/issues](https://github.com/dominiek/promptcellar-for-claude-code/issues).
