# Vendored gitleaks default config

This directory contains a verbatim copy of the [gitleaks](https://github.com/gitleaks/gitleaks) default configuration TOML, which Promptcellar's log-redaction baseline parses and evaluates against every prompt.

## What's here

- `gitleaks.toml` — the upstream `config/gitleaks.toml`, embedded into the Promptcellar binary at build time via `//go:embed`.
- `LICENSE` — gitleaks' MIT license.
- `VERSION.txt` — the commit + date the TOML was pulled from upstream.

## Why vendor

- **Reproducibility.** A user who runs `make build` against any commit of this repo gets exactly the rules that commit was tested against — not whatever happens to be on `master` upstream that day.
- **Offline / no-network builds.** No network call during compile.
- **Audit trail.** When a baseline rule changes, the diff shows up in this repo's commit history and is reviewable.

## License notice

Gitleaks is © 2019 Zachary Rice, MIT-licensed. The full license text is in `LICENSE`. Per its terms, the copyright notice must be included in distributions; that obligation is satisfied by shipping this directory inside the Promptcellar binary's source tree.

## Updating

To pull a newer rule set:

```sh
curl -fsSL https://raw.githubusercontent.com/gitleaks/gitleaks/master/config/gitleaks.toml \
  -o internal/plfignore/vendor/gitleaks/gitleaks.toml
curl -fsSL https://raw.githubusercontent.com/gitleaks/gitleaks/master/LICENSE \
  -o internal/plfignore/vendor/gitleaks/LICENSE
```

Update `VERSION.txt` with the new commit SHA + date, run `make test-all` to confirm nothing in the parser broke, and commit. Bump the plugin version too if the catalog changed materially.
